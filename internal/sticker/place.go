// Package sticker 把一个贴纸 (一个 .rm 笔迹文件, 坐标以贴纸中心为原点) 追加进笔记某一页的 .rm。
//
// 只在文件末尾追加新的笔画块, 页面原有字节一个不动。新笔画:
//   - 编号用本机作者 (1) 下比页面里所有编号都大的计数, 不和已有条目冲突;
//   - 接在图层里最后一笔后面 (CRDT 序列的 left = 上一笔), 显示时就是最上面一层;
//   - 坐标按 落点 + 缩放 变换, 每点线宽一起缩放。
//
// 写之前要让 xochitl 先把这页存盘并关闭 (DocumentView.close), 写完再打开, 否则内存里的旧版本会写回。
package sticker

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"

	rmscene "github.com/alexgorbatchev/go-rmscene"
)

// 新页面 (还没写过字) 的默认图层: 树节点 0:11 ("Layer 1")
var defaultLayer = rmscene.CrdtId{Part1: 0, Part2: 11}

type pageInfo struct {
	layer       rmscene.CrdtId
	last        rmscene.CrdtId // 图层里文件顺序上最后一笔 (追加时的 left)
	maxCounter  uint64         // 作者 1 下最大的编号
	lineVersion uint8
}

func scan(rm []byte) (pageInfo, []*rmscene.SceneLineItemBlock, error) {
	info := pageInfo{layer: defaultLayer, lineVersion: 2}
	if err := rmscene.ValidateHeaderBytes(rm); err != nil {
		return info, nil, err
	}
	s := rmscene.NewDataStream(bytes.NewReader(rm[rmscene.HeaderLength:]))
	var lines []*rmscene.SceneLineItemBlock
	seenLine := false
	bump := func(id rmscene.CrdtId) {
		if id.Part1 == 1 && id.Part2 > info.maxCounter {
			info.maxCounter = id.Part2
		}
	}
	for {
		start := s.Tell()
		b, err := rmscene.ReadBlock(s)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// 认不得的新块 (3.x 的图片块等) 读失败时 ReadBlock 已跳到块尾, 继续
			if s.Tell() == start {
				return info, nil, err
			}
			continue
		}
		switch v := b.(type) {
		case *rmscene.SceneLineItemBlock:
			bump(v.Item.ItemID)
			if !seenLine || v.ParentID == info.layer {
				info.layer = v.ParentID
				info.last = v.Item.ItemID
				seenLine = true
			}
			lines = append(lines, v)
		case *rmscene.SceneGroupItemBlock:
			bump(v.Item.ItemID)
		case *rmscene.TreeNodeBlock:
			bump(v.Group.NodeID)
		}
	}
	return info, lines, nil
}

// Place 返回追加了贴纸笔画的新页面文件, 以及加了几笔。x, y 是贴纸中心落在页面上的位置
// (页面坐标: x 以中线为 0, y 从页顶往下), scale 是缩放 (1 = 贴纸文件里的原尺寸)。
func Place(page, sticker []byte, x, y, scale float32) ([]byte, int, error) {
	if scale <= 0 {
		scale = 1
	}
	pi, _, err := scan(page)
	if err != nil {
		return nil, 0, fmt.Errorf("页面文件: %w", err)
	}
	_, src, err := scan(sticker)
	if err != nil {
		return nil, 0, fmt.Errorf("贴纸文件: %w", err)
	}
	var buf bytes.Buffer
	w := rmscene.NewDataWriter(&buf)
	next := pi.maxCounter + 1
	left := pi.last
	n := 0
	for _, b := range src {
		ln := b.Item.Value
		if ln == nil || len(ln.Points) == 0 {
			continue // 贴纸文件里被删掉的笔
		}
		pts := make([]rmscene.Point, len(ln.Points))
		for i, p := range ln.Points {
			p.X = p.X*scale + x
			p.Y = p.Y*scale + y
			p.Width = uint16(math.Max(1, math.Min(65535, math.Round(float64(p.Width)*float64(scale)))))
			pts[i] = p
		}
		nl := *ln
		nl.Points = pts
		nl.MoveID = nil
		id := rmscene.CrdtId{Part1: 1, Part2: next}
		blk := &rmscene.SceneLineItemBlock{
			ParentID: pi.layer,
			Item: rmscene.Item[*rmscene.Line]{
				ItemID: id, LeftID: left, RightID: rmscene.CrdtId{}, Value: &nl,
			},
		}
		if err := w.WriteSceneLineItemBlock(blk, pi.lineVersion); err != nil {
			return nil, 0, err
		}
		left = id
		next++
		n++
	}
	out := append(append([]byte(nil), page...), buf.Bytes()...)
	return out, n, nil
}
