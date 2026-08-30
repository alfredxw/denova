package interactive

import (
	"reflect"
	"testing"
)

func TestParseLoreReferencesToleratesFreeFormPlanText(t *testing.T) {
	want := []string{"沈凝", "旧车站"}
	got := ParseLoreReferences("先让 [[沈凝]] 调查 [[旧车站]]，随后再次确认 [[沈凝]]；忽略 [[]] 和未闭合 [[引用。")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("references = %#v, want %#v", got, want)
	}
}
