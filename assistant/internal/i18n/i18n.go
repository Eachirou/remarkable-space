// Package i18n 是助手的双语开关: 界面语言由前端按系统语言判断 (可在设置里改), 再告诉 Go 端。
//
// 不做词典, 每条文案就地写两种: T("中文", "English")。助手的文案都是一次性的句子,
// 查表反而让人读代码时看不到它到底说了什么。设备端脚本通过 UI_LANG 环境变量拿到同一个值。
package i18n

import "sync/atomic"

var english atomic.Bool

// Set 切换语言; "en" 为英文, 其他一律中文。
func Set(lang string) { english.Store(lang == "en") }

// Lang 给设备端脚本用: "zh" / "en"。
func Lang() string {
	if english.Load() {
		return "en"
	}
	return "zh"
}

// T 按当前语言选一条。
func T(zh, en string) string {
	if english.Load() {
		return en
	}
	return zh
}
