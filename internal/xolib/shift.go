package xolib

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	rmscene "github.com/alexgorbatchev/go-rmscene"
)

// Shift 把 .rm 里所有笔画点的坐标平移 (dx, dy), 返回新文件。只改每个点的 x/y 那 8 个字节,
// 其余字节 (CRDT 编号、时间戳、压力宽度…) 一个不动 —— 文件仍是 xochitl 自己写的那份结构。
//
// 为什么要平移: 原生笔输入层给的是整块屏幕的坐标 (实测比例 1:1, 原点在屏幕左上角),
// 而画布可能在屏幕任何位置 (日历格子 / 整页)。正本里存画布坐标, 拷进载体时加上画布当前在
// 屏幕上的位置, 拷回时减掉 —— 画布换了位置照样画得对, 写字时橡皮/撤销也和屏幕坐标对得上。
// 笔输入层本身的 setTransform 要 QTransform, QML 造不出来, 只能在这里换。
//
// 文件布局照 go-rmscene 的读法 (blocks.go readSceneLineItemBlock / readLine / readPoint):
// 块头 [u32 长度][u8][u8 最低版本][u8 当前版本][u8 类型]; 类型 5 是笔画块, 里面依次是
// id 1-4、删除长度 5、子块 6 (条目类型 u8, 笔型 1, 颜色 2, 粗细 3, 起始长度 4, 点数组子块 5)。
// 点: 当前版本 >=2 每点 14 字节 (x f32, y f32, 速度 u16, 宽度 u16, 方向 u8, 压力 u8), 版本 1 是 24 字节; 都是 x,y 打头。
func Shift(rm []byte, dx, dy float32) ([]byte, error) {
	if dx == 0 && dy == 0 {
		return rm, nil
	}
	out := append([]byte(nil), rm...)
	r := bytes.NewReader(out)
	s := rmscene.NewDataStream(r)
	if _, err := s.ReadBytes(43); err != nil { // 文件头 "reMarkable .lines file, version=6" + 补齐
		return nil, fmt.Errorf("文件头: %w", err)
	}
	for {
		size, err := s.ReadUint32()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return out, nil
			}
			return nil, err
		}
		hdr, err := s.ReadBytes(4) // 未知, 最低版本, 当前版本, 类型
		if err != nil {
			return nil, err
		}
		start := s.Tell()
		end := start + int64(size)
		if hdr[3] == rmscene.BlockTypeSceneLine {
			if err := shiftLineBlock(s, out, hdr[2], end, dx, dy); err != nil {
				return nil, fmt.Errorf("笔画块 @%d: %w", start, err)
			}
		}
		if _, err := s.Seek(end, io.SeekStart); err != nil {
			return nil, err
		}
	}
}

func shiftLineBlock(s *rmscene.DataStream, out []byte, version uint8, end int64, dx, dy float32) error {
	for i := 1; i <= 4; i++ {
		if _, err := s.ReadId(i); err != nil {
			return err
		}
	}
	if _, err := s.ReadTaggedInt(5); err != nil {
		return err
	}
	if s.Tell() >= end || !s.CheckTag(6, rmscene.TagLength4) {
		return nil // 被删掉的笔画只剩占位, 没有点
	}
	if _, _, err := s.ReadTag(6, rmscene.TagLength4); err != nil {
		return err
	}
	if _, err := s.ReadUint32(); err != nil {
		return err
	}
	if _, err := s.ReadUint8(); err != nil { // 条目类型 (笔画 = 3)
		return err
	}
	if _, err := s.ReadTaggedInt(1); err != nil {
		return err
	}
	if _, err := s.ReadTaggedInt(2); err != nil {
		return err
	}
	if _, err := s.ReadTaggedDouble(3); err != nil {
		return err
	}
	if _, err := s.ReadTaggedFloat(4); err != nil {
		return err
	}
	if _, _, err := s.ReadTag(5, rmscene.TagLength4); err != nil {
		return err
	}
	n, err := s.ReadUint32()
	if err != nil {
		return err
	}
	size := int64(14)
	if version == 1 {
		size = 24
	}
	if int64(n)%size != 0 {
		return fmt.Errorf("点数据 %d 字节不是 %d 的整数倍", n, size)
	}
	base := s.Tell()
	if base+int64(n) > int64(len(out)) || base+int64(n) > end {
		return fmt.Errorf("点数据越界")
	}
	for off := base; off < base+int64(n); off += size {
		addF32(out[off:], dx)
		addF32(out[off+4:], dy)
	}
	return nil
}

func addF32(b []byte, d float32) {
	v := math.Float32frombits(binary.LittleEndian.Uint32(b))
	binary.LittleEndian.PutUint32(b, math.Float32bits(v+d))
}
