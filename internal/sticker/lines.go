package sticker

// 贴纸文件里的笔画: go-rmscene 不把笔画块末尾的自定义颜色 (tag 8 = 字节 84 01, 随后小端 BGRA)
// 解进 Line, 只留在 ExtraValueData 里, 这里补读。2026-10-21 实测: 笔的 lineColor=9 + lineArgbCode=该值就能画任意 RGB,
// 文件里存的也是 color=9 + 这 4 个字节。

import (
	"encoding/binary"

	rmscene "github.com/alexgorbatchev/go-rmscene"
)

type Stroke struct {
	*rmscene.Line
	ARGB   uint32 // 自定义颜色 0xAARRGGBB
	HasRGB bool
}

// Strokes 全部笔画 (含自定义颜色), 不含荧光笔
func Strokes(sc *rmscene.Scene) []Stroke {
	var out []Stroke
	for _, b := range sc.Blocks {
		lb, ok := b.(*rmscene.SceneLineItemBlock)
		if !ok || lb.Item.Value == nil || lb.Item.Value.Tool.IsHighlighter() {
			continue
		}
		st := Stroke{Line: lb.Item.Value}
		if ex := lb.ExtraValueData; len(ex) >= 6 && ex[0] == 0x84 && ex[1] == 0x01 {
			st.ARGB = binary.LittleEndian.Uint32(ex[2:6])
			st.HasRGB = true
		}
		out = append(out, st)
	}
	return out
}
