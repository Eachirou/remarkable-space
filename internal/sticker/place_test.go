package sticker

import (
	"os"
	"testing"

	rmscene "github.com/alexgorbatchev/go-rmscene"
)

// 真机页面 + 贴纸: STICKER_PAGE / STICKER_FILE 指向样本 (没设就跳过)
func TestPlaceRealFiles(t *testing.T) {
	pp, sp := os.Getenv("STICKER_PAGE"), os.Getenv("STICKER_FILE")
	if pp == "" || sp == "" {
		t.Skip("样本未设")
	}
	page, _ := os.ReadFile(pp)
	st, _ := os.ReadFile(sp)
	out, n, err := Place(page, st, -440, 1650, 1)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := rmscene.Parse(page)
	after, err := rmscene.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Lines()) != len(before.Lines())+n {
		t.Fatalf("笔数 %d → %d, 加了 %d", len(before.Lines()), len(after.Lines()), n)
	}
	ids := map[rmscene.CrdtId]bool{}
	_, all, _ := scan(out)
	for _, b := range all {
		if ids[b.Item.ItemID] {
			t.Fatalf("编号重复 %v", b.Item.ItemID)
		}
		ids[b.Item.ItemID] = true
	}
	if os.Getenv("STICKER_OUT") != "" {
		os.WriteFile(os.Getenv("STICKER_OUT"), out, 0o644)
	}
	t.Logf("页面 %d 笔 → %d 笔 (+%d), %d → %d 字节", len(before.Lines()), len(after.Lines()), n, len(page), len(out))
}
