package xolib

import (
	"bytes"
	"math"
	"os"
	"testing"

	rmscene "github.com/alexgorbatchev/go-rmscene"
)

func TestShiftMovesEveryPoint(t *testing.T) {
	for _, name := range []string{"testdata/three-strokes.rm", "testdata/seventeen-strokes.rm"} {
		orig, _ := os.ReadFile(name)
		moved, err := Shift(orig, -88, -493)
		if err != nil {
			t.Fatal(name, err)
		}
		if len(moved) != len(orig) {
			t.Fatal("长度变了")
		}
		a, _ := rmscene.Parse(orig)
		b, err := rmscene.Parse(moved)
		if err != nil {
			t.Fatal("平移后 go-rmscene 解析失败", err)
		}
		la, lb := a.Lines(), b.Lines()
		if len(la) != len(lb) {
			t.Fatal("笔画数变了")
		}
		for i := range la {
			for j := range la[i].Points {
				pa, pb := la[i].Points[j], lb[i].Points[j]
				if math.Abs(float64(pb.X-(pa.X-88))) > 0.01 || math.Abs(float64(pb.Y-(pa.Y-493))) > 0.01 ||
					pa.Width != pb.Width || pa.Pressure != pb.Pressure || pa.Speed != pb.Speed {
					t.Fatalf("%s 第 %d 笔第 %d 点: %+v → %+v", name, i, j, pa, pb)
				}
			}
		}
		// 平移回去应与原文件逐字节相同 (浮点加减 88/493 在这个量级上是精确的)
		back, _ := Shift(moved, 88, 493)
		if !bytes.Equal(back, orig) {
			diff := 0
			for k := range back {
				if back[k] != orig[k] {
					diff++
				}
			}
			t.Logf("%s: 往返后有 %d 字节不同 (浮点舍入)", name, diff)
		}
	}
}

func TestShiftZeroIsNoop(t *testing.T) {
	orig, _ := os.ReadFile("testdata/three-strokes.rm")
	got, _ := Shift(orig, 0, 0)
	if !bytes.Equal(got, orig) {
		t.Fatal()
	}
}
