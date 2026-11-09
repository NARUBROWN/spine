package container

import (
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type objectDB struct{}
type objectRepository struct {
	db    *objectDB
	state int
}

func (r *objectRepository) Constructor(db *objectDB) *objectRepository {
	r.db = db
	r.state++
	return r
}

type missingConstructor struct{}
type wrongSelf struct{}

func (*wrongSelf) Constructor() *objectRepository { return nil }

type variadicConstructor struct{}

func (*variadicConstructor) Constructor(...string) *variadicConstructor { return nil }

type fallibleConstructor struct{}

func (*fallibleConstructor) Constructor() (*fallibleConstructor, error) { return nil, nil }

type nilConstructor struct{}

func (*nilConstructor) Constructor() *nilConstructor { return nil }

type panicConstructor struct{}

func (*panicConstructor) Constructor() *panicConstructor { panic("constructor failed") }

func TestObjectConstructorIsolationAndSingleton(t *testing.T) {
	prototype := &objectRepository{state: 99}
	factory, err := ObjectConstructor(prototype)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	var previous *objectRepository
	for range 2 {
		c := New()
		if err := c.RegisterConstructor(func() *objectDB { calls.Add(1); return &objectDB{} }); err != nil {
			t.Fatal(err)
		}
		if err := c.RegisterConstructor(factory); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		values := make(chan *objectRepository, 20)
		for range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				v, e := c.Resolve(reflect.TypeFor[*objectRepository]())
				if e != nil {
					t.Error(e)
					return
				}
				values <- v.(*objectRepository)
			}()
		}
		wg.Wait()
		close(values)
		var first *objectRepository
		for v := range values {
			if first == nil {
				first = v
			}
			if v != first || v.db == nil || v.state != 1 {
				t.Fatalf("invalid managed object: %+v", v)
			}
		}
		if first == previous {
			t.Fatal("prepared containers shared an object")
		}
		previous = first
	}
	if calls.Load() != 2 || prototype.state != 99 || prototype.db != nil {
		t.Fatalf("calls=%d prototype=%+v", calls.Load(), prototype)
	}
}

func TestObjectConstructorContracts(t *testing.T) {
	for _, value := range []any{nil, missingConstructor{}, &missingConstructor{}, &wrongSelf{}, &variadicConstructor{}, &fallibleConstructor{}, func() *objectDB { return nil }} {
		if _, err := ObjectConstructor(value); err == nil {
			t.Fatalf("accepted %T", value)
		}
	}
	if _, err := ObjectConstructor((*objectRepository)(nil)); err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{&nilConstructor{}, &panicConstructor{}} {
		f, err := ObjectConstructor(value)
		if err != nil {
			t.Fatal(err)
		}
		c := New()
		if err = c.RegisterConstructor(f); err != nil {
			t.Fatal(err)
		}
		if _, err = c.Resolve(reflect.TypeOf(value)); err == nil {
			t.Fatalf("accepted invalid result from %T", value)
		}
	}
}

func TestObjectConstructorPreflightHasNoSideEffects(t *testing.T) {
	var calls atomic.Int32
	factory, _ := ObjectConstructor(&objectRepository{})
	db := func() *objectDB { calls.Add(1); return &objectDB{} }
	for _, tc := range []struct {
		constructors []any
		want         string
	}{
		{[]any{factory}, "no constructor"},
		{[]any{factory, db, db}, "duplicate"},
		{[]any{func(*objectRepository) *objectDB { return nil }, factory}, "circular"},
	} {
		err := ValidateConstructors(tc.constructors)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("want %s: %v", tc.want, err)
		}
	}
	if err := ValidateConstructors([]any{factory, db}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("preflight called a constructor")
	}
}

type objectMultiple struct {
	db   *objectDB
	repo *objectRepository
}

func (*objectMultiple) Constructor(db *objectDB, r *objectRepository) *objectMultiple {
	return &objectMultiple{db, r}
}
func TestObjectConstructorMultipleDependencies(t *testing.T) {
	c := New()
	db := &objectDB{}
	if err := c.RegisterConstructor(func() *objectDB { return db }); err != nil {
		t.Fatal(err)
	}
	for _, prototype := range []any{&objectRepository{}, &objectMultiple{}} {
		factory, err := ObjectConstructor(prototype)
		if err != nil {
			t.Fatal(err)
		}
		if err = c.RegisterConstructor(factory); err != nil {
			t.Fatal(err)
		}
	}
	value, err := c.Resolve(reflect.TypeFor[*objectMultiple]())
	if err != nil {
		t.Fatal(err)
	}
	result := value.(*objectMultiple)
	if result.db != db || result.repo.db != db {
		t.Fatal("dependencies did not share the managed instance")
	}
}
