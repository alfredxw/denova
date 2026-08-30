package lore

import (
	"fmt"
	"strings"
)

// Lore names reserve double brackets because Game Agent plans use [[name]]
// as an explicit, human-readable reference to a Lore item.
func validateLoreReferenceName(name string) error {
	if strings.Contains(name, "[[") || strings.Contains(name, "]]") {
		return fmt.Errorf("资料名称不能包含引用保留符号 [[ 或 ]] / Lore names cannot contain reserved reference markers [[ or ]]")
	}
	return nil
}
