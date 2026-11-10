package execution

import (
	"sync"
	"testing"
)

func TestStoreReleaseDetachesValuesAndClosesWrites(t *testing.T) {
	var store Store
	store.Set("payload", make([]byte, 1024))
	store.ReleaseStore()
	store.Set("late", make([]byte, 1024))
	if store.values != nil {
		t.Fatal("완료된 저장소가 값의 참조를 보관합니다")
	}
	for _, key := range []string{"payload", "late"} {
		if value, ok := store.Get(key); ok || value != nil {
			t.Fatalf("완료된 저장소에 값이 남았습니다: %s", key)
		}
	}
	store.ReleaseStore()
}

func TestStoreConcurrentReadsWritesAndRelease(t *testing.T) {
	var store Store
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			for range 100 {
				store.Set("value", 1)
				store.Get("value")
			}
		})
	}
	store.ReleaseStore()
	group.Wait()
	if _, ok := store.Get("value"); ok {
		t.Fatal("완료 후 병렬 쓰기가 저장소를 다시 열었습니다")
	}
}
