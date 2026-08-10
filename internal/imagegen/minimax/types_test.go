package minimax

import "testing"

// TestFlexibleStringsUnmarshal 测试 flexibleStrings 兼容 JSON 字符串和数组。
func TestFlexibleStringsUnmarshal(t *testing.T) {
	// 单个字符串
	var f1 flexibleStrings
	if err := f1.UnmarshalJSON([]byte(`"abc"`)); err != nil {
		t.Fatalf("解析单个字符串失败: %v", err)
	}
	if len(f1) != 1 || f1[0] != "abc" {
		t.Errorf("单个字符串解析错误: %v", f1)
	}

	// 字符串数组
	var f2 flexibleStrings
	if err := f2.UnmarshalJSON([]byte(`["a", "b", "c"]`)); err != nil {
		t.Fatalf("解析数组失败: %v", err)
	}
	if len(f2) != 3 {
		t.Errorf("数组解析长度错误: %v", f2)
	}

	// 空字符串
	var f3 flexibleStrings
	if err := f3.UnmarshalJSON([]byte(`""`)); err != nil {
		t.Fatalf("解析空字符串失败: %v", err)
	}
	if len(f3) != 0 {
		t.Errorf("空字符串应为空数组: %v", f3)
	}
}
