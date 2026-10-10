package xolib

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newInk(t *testing.T) (*Ink, string) {
	t.Helper()
	root := t.TempDir()
	lib := New(filepath.Join(root, "xochitl"), "")
	return &Ink{Lib: lib, DataDir: filepath.Join(root, "data"), State: filepath.Join(root, "data", "shell", "ink.json")}, root
}

func TestRenderRealFiles(t *testing.T) {
	for name, want := range map[string]int{"three-strokes.rm": 3, "seventeen-strokes.rm": 17} {
		b, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		d, err := Render(b)
		if err != nil {
			t.Fatal(name, err)
		}
		if d.Strokes != want || len(d.Runs) < want {
			t.Fatalf("%s: strokes=%d runs=%d", name, d.Strokes, len(d.Runs))
		}
		for _, r := range d.Runs {
			if r.Width <= 0 || len(r.Points)%2 != 0 || len(r.Points) < 2 || !strings.HasPrefix(r.Color, "#") && !strings.HasPrefix(r.Color, "rgb") {
				t.Fatalf("%s: 坏线段 %+v", name, r)
			}
		}
		if d.Bounds[2] <= d.Bounds[0] || d.Bounds[3] <= d.Bounds[1] {
			t.Fatalf("%s: bounds %v", name, d.Bounds)
		}
	}
}

func TestInkLoadSaveCycle(t *testing.T) {
	k, _ := newInk(t)
	a, _ := os.ReadFile("testdata/three-strokes.rm")
	b, _ := os.ReadFile("testdata/seventeen-strokes.rm")

	// 新画布: 建载体 (在回收站), 载体页是空的
	s, err := k.Load("plan", "2026-10-09", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Created {
		t.Fatal("第一次应新建载体")
	}
	meta, _ := os.ReadFile(filepath.Join(k.Lib.Dir, s.CarrierID+".metadata"))
	if !strings.Contains(string(meta), `"parent": "trash"`) {
		t.Fatalf("载体应在回收站: %s", meta)
	}
	page := filepath.Join(k.Lib.Dir, s.CarrierID, s.PageID+".rm")
	if _, err := os.Stat(page); !os.IsNotExist(err) {
		t.Fatal("新画布载体页应为空")
	}
	// xochitl 写了几笔 → 关掉 → Save 拷回正本
	os.WriteFile(page, a, 0o644)
	if err := k.Save("plan", "2026-10-09"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(k.DataDir, "plan", "ink", "2026-10-09.rm")); !bytes.Equal(got, a) {
		t.Fatal("正本不对")
	}
	// 换另一块画布: 复用载体, 载体页变成那块的正本
	os.MkdirAll(filepath.Join(k.DataDir, "plan", "ink"), 0o755)
	os.WriteFile(filepath.Join(k.DataDir, "plan", "ink", "2026-10-10.rm"), b, 0o644)
	s2, _ := k.Load("plan", "2026-10-10", 0, 0)
	if s2.Created || s2.CarrierID != s.CarrierID {
		t.Fatal("应复用载体")
	}
	if got, _ := os.ReadFile(page); !bytes.Equal(got, b) {
		t.Fatal("载体页应换成 10-10 的正本")
	}
	// 写字时读 live 是载体页, 不读 live 是正本
	live, _ := k.Strokes("plan", "2026-10-10", true)
	if live.Strokes != 17 {
		t.Fatal(live.Strokes)
	}
	// 回到 10-09: 载体页换回 a
	k.Save("plan", "2026-10-10")
	k.Load("plan", "2026-10-09", 0, 0)
	if got, _ := os.ReadFile(page); !bytes.Equal(got, a) {
		t.Fatal("切回 10-09 不对")
	}
}

// 写到一半没 Save 就去开别的画布 (空间崩了): 没拷回的那份要先存回原主
func TestInkRecoversUnsaved(t *testing.T) {
	k, _ := newInk(t)
	a, _ := os.ReadFile("testdata/three-strokes.rm")
	s, _ := k.Load("plan", "d1", 0, 0)
	os.WriteFile(filepath.Join(k.Lib.Dir, s.CarrierID, s.PageID+".rm"), a, 0o644)
	// 没 Save, 直接开 d2
	if _, err := k.Load("plan", "d2", 0, 0); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(k.DataDir, "plan", "ink", "d1.rm")); !bytes.Equal(got, a) {
		t.Fatal("d1 写到一半的笔迹丢了")
	}
}

// 回收站被清空 (载体没了) → 重建; 被恢复到「我的文件」→ 先存回再另建
func TestInkCarrierRecreated(t *testing.T) {
	k, _ := newInk(t)
	a, _ := os.ReadFile("testdata/three-strokes.rm")
	s, _ := k.Load("plan", "d1", 0, 0)
	os.WriteFile(filepath.Join(k.Lib.Dir, s.CarrierID, s.PageID+".rm"), a, 0o644)
	// 用户把载体恢复到「我的文件」
	mp := filepath.Join(k.Lib.Dir, s.CarrierID+".metadata")
	m, _ := os.ReadFile(mp)
	os.WriteFile(mp, bytes.Replace(m, []byte(`"parent": "trash"`), []byte(`"parent": ""`), 1), 0o644)
	s2, err := k.Load("plan", "d2", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !s2.Created || s2.CarrierID == s.CarrierID {
		t.Fatal("应另建载体")
	}
	if got, _ := os.ReadFile(filepath.Join(k.DataDir, "plan", "ink", "d1.rm")); !bytes.Equal(got, a) {
		t.Fatal("换载体前没把 d1 存回")
	}
	// 回收站清空: 载体文件没了
	os.Remove(filepath.Join(k.Lib.Dir, s2.CarrierID+".metadata"))
	k.Save("plan", "d2")
	s3, _ := k.Load("plan", "d3", 0, 0)
	if !s3.Created {
		t.Fatal("载体没了应重建")
	}
}

func TestInkBadKeys(t *testing.T) {
	k, _ := newInk(t)
	for _, bad := range [][2]string{{"../x", "a"}, {"plan", "../../etc"}, {"plan", ""}, {"", "a"}, {"plan", "a/b"}} {
		if _, err := k.Load(bad[0], bad[1], 0, 0); err != ErrBadKey {
			t.Errorf("%v: %v", bad, err)
		}
	}
}

func TestInkSaveWrongSession(t *testing.T) {
	k, _ := newInk(t)
	k.Load("plan", "d1", 0, 0)
	if err := k.Save("plan", "d2"); err == nil {
		t.Fatal("载体里不是 d2, 不应能存")
	}
}

// 同一块画布写到一半被打断, 再打开同一块: 载体里的新笔迹不能被旧正本覆盖
func TestInkResumeSameCanvas(t *testing.T) {
	k, _ := newInk(t)
	a, _ := os.ReadFile("testdata/three-strokes.rm")
	b, _ := os.ReadFile("testdata/seventeen-strokes.rm")
	os.MkdirAll(filepath.Join(k.DataDir, "plan", "ink"), 0o755)
	os.WriteFile(filepath.Join(k.DataDir, "plan", "ink", "d1.rm"), a, 0o644) // 旧正本
	s, _ := k.Load("plan", "d1", 0, 0)
	page := filepath.Join(k.Lib.Dir, s.CarrierID, s.PageID+".rm")
	os.WriteFile(page, b, 0o644) // 写了新笔迹, 没 Save
	k.Load("plan", "d1", 0, 0)
	if got, _ := os.ReadFile(page); !bytes.Equal(got, b) {
		t.Fatal("新笔迹被旧正本覆盖了")
	}
	k.Save("plan", "d1")
	if got, _ := os.ReadFile(filepath.Join(k.DataDir, "plan", "ink", "d1.rm")); !bytes.Equal(got, b) {
		t.Fatal("存回的不是新笔迹")
	}
}

// 画布在屏幕 (88,493): 正本存画布坐标, 载体里是屏幕坐标, 存回去又是画布坐标
func TestInkOriginRoundTrip(t *testing.T) {
	k, _ := newInk(t)
	a, _ := os.ReadFile("testdata/three-strokes.rm")
	os.MkdirAll(filepath.Join(k.DataDir, "plan", "ink"), 0o755)
	os.WriteFile(filepath.Join(k.DataDir, "plan", "ink", "d1.rm"), a, 0o644)
	s, _ := k.Load("plan", "d1", 88, 493)
	page, _ := os.ReadFile(filepath.Join(k.Lib.Dir, s.CarrierID, s.PageID+".rm"))
	want, _ := Shift(a, 88, 493)
	if !bytes.Equal(page, want) {
		t.Fatal("载体里应是屏幕坐标")
	}
	live, _ := k.Strokes("plan", "d1", true)
	stored, _ := k.Strokes("plan", "d1", false)
	if !near(live.Bounds, stored.Bounds) {
		t.Fatalf("写字时读回的应换回画布坐标: %v vs %v", live.Bounds, stored.Bounds)
	}
	k.Save("plan", "d1")
	got, _ := os.ReadFile(filepath.Join(k.DataDir, "plan", "ink", "d1.rm"))
	if d, _ := Render(got); !near(d.Bounds, stored.Bounds) {
		t.Fatalf("存回的应是画布坐标: %v vs %v", d.Bounds, stored.Bounds)
	}
}

// 浮点加减来回会有 1e-5 量级的舍入
func near(a, b [4]float64) bool {
	for i := range a {
		if d := a[i] - b[i]; d > 0.01 || d < -0.01 {
			return false
		}
	}
	return true
}
