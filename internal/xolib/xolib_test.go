package xolib

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestImportWritesFilesAndQueues(t *testing.T) {
	dir := t.TempDir()
	l := New(dir, "")
	e, err := l.ImportDocument(strings.NewReader("%PDF-1.4 x"), "我的书, 第2版", ".PDF", "")
	if err != nil {
		t.Fatal(err)
	}
	if !uuidRe.MatchString(e.ID) || e.Type != "pdf" {
		t.Fatalf("%+v", e)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, e.ID+".pdf")); string(b) != "%PDF-1.4 x" {
		t.Fatal("本体不对")
	}
	var meta map[string]any
	b, _ := os.ReadFile(filepath.Join(dir, e.ID+".metadata"))
	if json.Unmarshal(b, &meta) != nil || meta["visibleName"] != "我的书, 第2版" || meta["type"] != "DocumentType" || meta["parent"] != "" {
		t.Fatalf("metadata: %s", b)
	}
	var content map[string]any
	b, _ = os.ReadFile(filepath.Join(dir, e.ID+".content"))
	if json.Unmarshal(b, &content) != nil || content["fileType"] != "pdf" {
		t.Fatalf("content: %s", b)
	}
	// 不留临时文件
	if m, _ := filepath.Glob(filepath.Join(dir, ".xolib-*")); len(m) != 0 {
		t.Fatal("临时文件没清掉", m)
	}
	if l.Queue.Pending() != 1 {
		t.Fatal("没排队")
	}
}

func TestRejectsOtherTypes(t *testing.T) {
	l := New(t.TempDir(), "")
	if _, err := l.ImportDocument(strings.NewReader("x"), "a", "exe", ""); err != ErrBadType {
		t.Fatal(err)
	}
}

func TestNextWaitsAndWakes(t *testing.T) {
	q := NewQueue("")
	got := make(chan []Job, 1)
	go func() { got <- q.Next(context.Background(), 5*time.Second) }()
	time.Sleep(50 * time.Millisecond)
	if !q.Active() {
		t.Fatal("挂着长连接时应算开着")
	}
	q.Enqueue(Job{Op: OpLoad, ID: "a"})
	select {
	case jobs := <-got:
		if len(jobs) != 1 || jobs[0].ID != "a" {
			t.Fatal(jobs)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("没被叫醒")
	}
	if q.Pending() != 0 {
		t.Fatal("取走后应出队")
	}
}

func TestNextTimeout(t *testing.T) {
	q := NewQueue("")
	t0 := time.Now()
	if jobs := q.Next(context.Background(), 100*time.Millisecond); jobs != nil {
		t.Fatal(jobs)
	}
	if time.Since(t0) < 90*time.Millisecond {
		t.Fatal("没等够")
	}
}

func TestFailedJobRetriesLater(t *testing.T) {
	q := NewQueue("")
	q.Enqueue(Job{Op: OpLoad, ID: "a"})
	q.Next(context.Background(), 0)
	q.Done(Result{Op: OpLoad, ID: "a", OK: false, Error: "找不到文件库"})
	// 失败的活不能立刻再给出去 (否则外壳和服务端空转)
	if jobs := q.Next(context.Background(), 50*time.Millisecond); jobs != nil {
		t.Fatal("失败后立刻又给出去了", jobs)
	}
	q.now = func() time.Time { return time.Now().Add(RetryDelay + time.Second) }
	if jobs := q.Next(context.Background(), 0); len(jobs) != 1 || !jobs[0].NotBefore.IsZero() {
		t.Fatal("过了重试时间应能取到", jobs)
	}
}

func TestWaitGetsResult(t *testing.T) {
	q := NewQueue("")
	go func() {
		time.Sleep(30 * time.Millisecond)
		q.Done(Result{Op: OpLoad, ID: "a", OK: true})
	}()
	r, ok := q.Wait("a", time.Second)
	if !ok || !r.OK {
		t.Fatal(r, ok)
	}
	if _, ok := q.Wait("b", 30*time.Millisecond); ok {
		t.Fatal("应超时")
	}
}

func TestActiveAndBye(t *testing.T) {
	q := NewQueue("")
	if q.Active() {
		t.Fatal("没来过应算关着")
	}
	q.Next(context.Background(), 0)
	if !q.Active() {
		t.Fatal("刚取过应算开着")
	}
	q.Close()
	if q.Active() {
		t.Fatal("说了关就是关")
	}
	q.Next(context.Background(), 0)
	if !q.Active() {
		t.Fatal("重新打开空间应恢复")
	}
	q.now = func() time.Time { return time.Now().Add(ActiveWindow + time.Second) }
	if q.Active() {
		t.Fatal("太久没来应算关着")
	}
}

// 写完文件、还没通知时 space-server 重启: 待通知的不能丢
func TestPendingSurvivesRestart(t *testing.T) {
	f := filepath.Join(t.TempDir(), "pending.json")
	NewQueue(f).Enqueue(Job{Op: OpLoad, ID: "a"})
	q2 := NewQueue(f)
	if jobs := q2.Next(context.Background(), 0); len(jobs) != 1 || jobs[0].ID != "a" {
		t.Fatal(jobs)
	}
	if NewQueue(f).Pending() != 0 {
		t.Fatal("取走后落盘也应清掉")
	}
}

func TestHTTP(t *testing.T) {
	l := New(t.TempDir(), "")
	mux := http.NewServeMux()
	l.Mount(mux)
	do := func(method, url, body string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(method, url, strings.NewReader(body)))
		return rr
	}
	if rr := do("GET", "/space/library/next?wait=0", ""); rr.Code != 204 {
		t.Fatal(rr.Code)
	}
	e, _ := l.ImportDocument(strings.NewReader("x"), "a", "epub", "")
	rr := do("GET", "/space/library/next?wait=0", "")
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), e.ID) {
		t.Fatal(rr.Code, rr.Body)
	}
	if rr := do("POST", "/space/library/done", `{"op":"load","id":"`+e.ID+`","ok":true}`); rr.Code != 200 {
		t.Fatal(rr.Code)
	}
	if rr := do("GET", "/space/library/status", ""); !strings.Contains(rr.Body.String(), `"open":true`) {
		t.Fatal(rr.Body)
	}
	do("POST", "/space/library/bye", "")
	if rr := do("GET", "/space/library/status", ""); !strings.Contains(rr.Body.String(), `"open":false`) {
		t.Fatal(rr.Body)
	}
}

func TestCreateFolderAndNotebook(t *testing.T) {
	dir := t.TempDir()
	l := New(dir, "")
	f, err := l.CreateFolder("空间", "")
	if err != nil {
		t.Fatal(err)
	}
	nb, err := l.CreateNotebook("空间画板", f.ID, "", false)
	if err != nil {
		t.Fatal(err)
	}
	var meta, content map[string]any
	b, _ := os.ReadFile(filepath.Join(dir, f.ID+".metadata"))
	json.Unmarshal(b, &meta)
	if meta["type"] != "CollectionType" || meta["visibleName"] != "空间" {
		t.Fatalf("folder: %s", b)
	}
	b, _ = os.ReadFile(filepath.Join(dir, nb.ID+".metadata"))
	json.Unmarshal(b, &meta)
	if meta["parent"] != f.ID || meta["type"] != "DocumentType" {
		t.Fatalf("notebook meta: %s", b)
	}
	b, _ = os.ReadFile(filepath.Join(dir, nb.ID+".content"))
	json.Unmarshal(b, &content)
	pages := content["cPages"].(map[string]any)["pages"].([]any)
	if content["fileType"] != "notebook" || len(pages) != 1 || pages[0].(map[string]any)["id"] != nb.PageID {
		t.Fatalf("notebook content: %s", b)
	}
	if l.Queue.Pending() != 2 {
		t.Fatal("文件夹和笔记本都要排队加载")
	}
}

func TestHiddenNotebook(t *testing.T) {
	dir := t.TempDir()
	nb, _ := New(dir, "").CreateNotebook("载体", "", "", true)
	b, _ := os.ReadFile(filepath.Join(dir, nb.ID+".metadata"))
	if !strings.Contains(string(b), `"deleted": true`) {
		t.Fatalf("%s", b)
	}
}

// 3.29 起: 外壳报导入器模式后, 上传的文档进暂存目录 (显示名当文件名), 不碰数据目录。
func TestImporterModeStagesDocument(t *testing.T) {
	dir, staging := t.TempDir(), t.TempDir()
	l := New(dir, "")
	l.Staging = staging
	l.SetImporter(true)
	e, err := l.ImportDocument(strings.NewReader("%PDF"), "习题/答案", "pdf", "p1")
	if err != nil {
		t.Fatal(err)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Fatal("导入器模式不该写数据目录", ents)
	}
	jobs := l.Queue.Next(context.Background(), 0)
	if len(jobs) != 1 || jobs[0].Op != OpImport || jobs[0].Parent != "p1" {
		t.Fatal("应排一条 import", jobs)
	}
	want := filepath.Join(staging, e.ID, "习题／答案.pdf")
	if jobs[0].Path != want {
		t.Fatal("暂存路径", jobs[0].Path, want)
	}
	if b, _ := os.ReadFile(want); string(b) != "%PDF" {
		t.Fatal("暂存内容不对")
	}
	// 失败按原样 (带路径) 排回
	l.Queue.Done(Result{Op: OpImport, ID: e.ID, OK: false})
	l.Queue.now = func() time.Time { return time.Now().Add(RetryDelay + time.Second) }
	if again := l.Queue.Next(context.Background(), 0); len(again) != 1 || again[0].Path != want {
		t.Fatal("失败的 import 应带路径排回", again)
	}
}

// 升级到 3.29 前上传、一直加载不了的文档: 切到导入器模式时改成导入, 数据目录里不留副本;
// 文件夹没法导入, 丢掉 (留在盘上, 重启后可见)。
func TestSwitchToImporterConvertsStuckLoads(t *testing.T) {
	dir := t.TempDir()
	l := New(dir, "")
	l.Staging = t.TempDir()
	doc, _ := l.ImportDocument(strings.NewReader("%PDF"), "旧文档", "pdf", "")
	folder, _ := l.CreateFolder("文件夹", "")
	l.SetImporter(true)
	jobs := l.Queue.Next(context.Background(), 0)
	if len(jobs) != 1 || jobs[0].Op != OpImport || jobs[0].ID != doc.ID {
		t.Fatal("只应剩文档的 import", jobs)
	}
	if filepath.Base(jobs[0].Path) != "旧文档.pdf" {
		t.Fatal("暂存文件名", jobs[0].Path)
	}
	if m, _ := filepath.Glob(filepath.Join(dir, doc.ID+".*")); len(m) != 0 {
		t.Fatal("数据目录还留着文档", m)
	}
	if _, err := os.Stat(filepath.Join(dir, folder.ID+".metadata")); err != nil {
		t.Fatal("文件夹应留在盘上")
	}
	// 导入器模式下失败的 load 不再排回
	l.Queue.Enqueue(Job{Op: OpLoad, ID: "x"})
	l.Queue.Next(context.Background(), 0)
	l.Queue.Done(Result{Op: OpLoad, ID: "x", OK: false})
	l.Queue.now = func() time.Time { return time.Now().Add(RetryDelay + time.Second) }
	if j := l.Queue.Next(context.Background(), 0); j != nil {
		t.Fatal("导入器模式下 load 失败不该重试", j)
	}
}
