package interactive

import "reflect"

// SameContextCompactionMutation compares the semantic mutation while ignoring
// journal envelope fields assigned by the canonical Story store.
func SameContextCompactionMutation(actual, expected ContextCompactionEvent) bool {
	actual.V, expected.V = 0, 0
	actual.Type, expected.Type = "", ""
	actual.ParentID, expected.ParentID = "", ""
	actual.BranchID, expected.BranchID = "", ""
	actual.Ts, expected.Ts = "", ""
	actual.ExpectedParentID, expected.ExpectedParentID = nil, nil
	return reflect.DeepEqual(actual, expected)
}

// SameContextCompactionRemovalMutation compares the semantic removal while
// ignoring journal envelope fields assigned by the canonical Story store.
func SameContextCompactionRemovalMutation(actual, expected ContextCompactionRemovalEvent) bool {
	actual.V, expected.V = 0, 0
	actual.Type, expected.Type = "", ""
	actual.ParentID, expected.ParentID = "", ""
	actual.BranchID, expected.BranchID = "", ""
	actual.Ts, expected.Ts = "", ""
	actual.ExpectedParentID, expected.ExpectedParentID = nil, nil
	return reflect.DeepEqual(actual, expected)
}
