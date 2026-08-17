package configresource

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type testAdapter struct{ name string }

func (a testAdapter) Descriptor() Descriptor {
	return Descriptor{Name: a.name, Operations: []string{ReadList, ReadGet, ApplyCreate}}
}
func (a testAdapter) List(context.Context, ReadRequest) (any, error) {
	return NewCatalog([]string{"list"}), nil
}
func (a testAdapter) Get(context.Context, ReadRequest) (any, error) { return "get", nil }
func (a testAdapter) Apply(context.Context, Mutation) (any, error)  { return "applied", nil }

func TestRegistryRoutesAndSortsResources(t *testing.T) {
	registry, err := New(testAdapter{name: "zeta"}, testAdapter{name: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	descriptors, err := registry.Describe("")
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{descriptors[0].Name, descriptors[1].Name}; !reflect.DeepEqual(got, []string{"alpha", "zeta"}) {
		t.Fatalf("descriptor order = %v", got)
	}
	got, err := registry.Read(context.Background(), ReadRequest{Operation: ReadGet, Resource: "alpha", IDs: []string{"one"}})
	want := GetResult{
		Schema: "config.read.v1", Status: "success", Resource: "alpha",
		Items: []any{"get"}, Processed: 1, Total: 1,
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Read() = %#v, %v", got, err)
	}
}

func TestRegistryRequiresRevisionForUpdateAndDelete(t *testing.T) {
	registry, err := New(testAdapter{name: "resource"})
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []Mutation{
		{Operation: ApplyUpdate, Resource: "resource", ID: "one", Value: map[string]any{"name": "next"}},
		{Operation: ApplyDelete, Resource: "resource", ID: "one"},
	} {
		if _, err := registry.Apply(context.Background(), mutation); err == nil {
			t.Fatalf("Apply(%+v) accepted a mutation without revision", mutation)
		}
	}
}

func TestRegistryRejectsInvalidDescriptorContracts(t *testing.T) {
	for _, adapter := range []descriptorAdapter{
		{descriptor: Descriptor{Operations: []string{ReadList}}},
		{descriptor: Descriptor{Name: "resource"}},
	} {
		if _, err := New(adapter); err == nil {
			t.Fatalf("New(%+v) accepted an invalid descriptor", adapter)
		}
	}

	missingRevision := descriptorAdapter{descriptor: Descriptor{
		Name: "resource", Operations: []string{ReadList, ApplyUpdate},
	}}
	if _, err := New(missingRevision); err == nil {
		t.Fatal("New accepted update without revision_field")
	}
	unknownOperation := descriptorAdapter{descriptor: Descriptor{
		Name: "resource", Operations: []string{"rename"},
	}}
	if _, err := New(unknownOperation); err == nil {
		t.Fatal("New accepted an unknown operation")
	}
}

func TestRegistryEnforcesDeclaredOperationsAndScopes(t *testing.T) {
	registry, err := New(descriptorAdapter{descriptor: Descriptor{
		Name: "resource", Scopes: []string{"user"}, Operations: []string{ReadList},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Read(context.Background(), ReadRequest{Operation: ReadGet, Resource: "resource", IDs: []string{"one"}}); err == nil {
		t.Fatal("Read accepted an undeclared get operation")
	}
	if _, err := registry.Read(context.Background(), ReadRequest{Operation: ReadList, Resource: "resource", Scope: "workspace"}); err == nil {
		t.Fatal("Read accepted an undeclared scope")
	}
}

type partialReadAdapter struct {
	catalog []string
}

func (adapter *partialReadAdapter) Descriptor() Descriptor {
	return Descriptor{Name: "partial", Operations: []string{ReadList, ReadGet}}
}
func (adapter *partialReadAdapter) List(context.Context, ReadRequest) (any, error) {
	return NewCatalog(adapter.catalog), nil
}
func (adapter *partialReadAdapter) Get(_ context.Context, request ReadRequest) (any, error) {
	id := request.IDs[0]
	switch id {
	case "missing":
		return nil, Missing(errors.New("not found"))
	case "broken":
		return nil, errors.New("storage unavailable")
	default:
		return map[string]string{"id": id}, nil
	}
}
func (*partialReadAdapter) Apply(context.Context, Mutation) (any, error) { return nil, nil }

func TestRegistryExactReadHasNoItemCountLimitAndReportsPartialFailures(t *testing.T) {
	adapter := &partialReadAdapter{}
	registry, err := New(adapter)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, 42)
	for index := 0; index < 40; index++ {
		ids = append(ids, fmt.Sprintf("item-%02d", index))
	}
	ids = append(ids, "missing", "broken")
	value, err := registry.Read(context.Background(), ReadRequest{Operation: ReadGet, Resource: "partial", IDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	result := value.(GetResult)
	if len(result.Items) != 40 || !reflect.DeepEqual(result.MissingIDs, []string{"missing"}) ||
		len(result.Failures) != 1 || result.Failures[0].ID != "broken" || result.Status != "partial" {
		t.Fatalf("partial exact read = %#v", result)
	}
	if _, err := registry.Read(context.Background(), ReadRequest{
		Operation: ReadGet, Resource: "partial", IDs: []string{"missing"},
	}); err == nil {
		t.Fatal("entirely missing exact read should fail")
	}
}

func TestRegistryListCursorIsStableAndDetectsCatalogChanges(t *testing.T) {
	adapter := &partialReadAdapter{catalog: []string{"one", "two", "three"}}
	registry, err := New(adapter)
	if err != nil {
		t.Fatal(err)
	}
	firstValue, err := registry.Read(context.Background(), ReadRequest{
		Operation: ReadList, Resource: "partial", Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	first := firstValue.(ListResult)
	if !first.Truncated || first.NextCursor == "" || !reflect.DeepEqual(first.Items, []any{"one", "two"}) {
		t.Fatalf("first catalog page = %#v", first)
	}
	secondValue, err := registry.Read(context.Background(), ReadRequest{
		Operation: ReadList, Resource: "partial", Limit: 2, Cursor: first.NextCursor,
	})
	if err != nil {
		t.Fatal(err)
	}
	second := secondValue.(ListResult)
	if second.Truncated || !reflect.DeepEqual(second.Items, []any{"three"}) {
		t.Fatalf("second catalog page = %#v", second)
	}
	adapter.catalog = append(adapter.catalog, "four")
	if _, err := registry.Read(context.Background(), ReadRequest{
		Operation: ReadList, Resource: "partial", Limit: 2, Cursor: first.NextCursor,
	}); err == nil || !strings.Contains(err.Error(), "catalog changed") {
		t.Fatalf("stale catalog cursor error = %v", err)
	}
}

func TestRegistryExactReadCursorCarriesPriorSuccessAcrossMissingPage(t *testing.T) {
	registry, err := New(&partialReadAdapter{})
	if err != nil {
		t.Fatal(err)
	}
	request := ReadRequest{
		Operation: ReadGet, Resource: "partial", IDs: []string{"one", "missing", "two"}, Limit: 1,
	}
	firstValue, err := registry.Read(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	first := firstValue.(GetResult)
	if len(first.Items) != 1 || first.NextCursor == "" {
		t.Fatalf("first exact page = %#v", first)
	}
	request.Cursor = first.NextCursor
	secondValue, err := registry.Read(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second := secondValue.(GetResult)
	if !reflect.DeepEqual(second.MissingIDs, []string{"missing"}) || second.NextCursor == "" {
		t.Fatalf("missing exact page = %#v", second)
	}
	request.Cursor = second.NextCursor
	thirdValue, err := registry.Read(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	third := thirdValue.(GetResult)
	if len(third.Items) != 1 || third.Truncated || third.NextCursor != "" {
		t.Fatalf("final exact page = %#v", third)
	}
}

type descriptorAdapter struct{ descriptor Descriptor }

func (adapter descriptorAdapter) Descriptor() Descriptor { return adapter.descriptor }
func (descriptorAdapter) List(context.Context, ReadRequest) (any, error) {
	return nil, nil
}
func (descriptorAdapter) Get(context.Context, ReadRequest) (any, error) { return nil, nil }
func (descriptorAdapter) Apply(context.Context, Mutation) (any, error)  { return nil, nil }
