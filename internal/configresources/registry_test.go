package configresources

import (
	"context"
	"reflect"
	"testing"
)

type testAdapter struct{ name string }

func (a testAdapter) Descriptor() Descriptor {
	return Descriptor{Name: a.name, Operations: []string{ReadList, ReadGet, ApplyCreate}}
}
func (a testAdapter) List(context.Context, ReadRequest) (any, error) { return []string{"list"}, nil }
func (a testAdapter) Get(context.Context, ReadRequest) (any, error)  { return []string{"get"}, nil }
func (a testAdapter) Apply(context.Context, Mutation) (any, error)   { return "applied", nil }

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
	if err != nil || !reflect.DeepEqual(got, []string{"get"}) {
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

type descriptorAdapter struct{ descriptor Descriptor }

func (adapter descriptorAdapter) Descriptor() Descriptor { return adapter.descriptor }
func (descriptorAdapter) List(context.Context, ReadRequest) (any, error) {
	return nil, nil
}
func (descriptorAdapter) Get(context.Context, ReadRequest) (any, error) { return nil, nil }
func (descriptorAdapter) Apply(context.Context, Mutation) (any, error)  { return nil, nil }
