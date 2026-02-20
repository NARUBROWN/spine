package ws

import "testing"

type registryTestController struct{}

func (c *registryTestController) Handle() {}

func TestRegistry_RegisterAndCopy(t *testing.T) {
	registry := NewRegistry()

	registry.Register("/ws/echo", (*registryTestController).Handle)

	got := registry.Registrations()
	if len(got) != 1 {
		t.Fatalf("등록 개수가 잘못되었습니다: %d", len(got))
	}
	if got[0].Path != "/ws/echo" {
		t.Fatalf("등록된 경로가 잘못되었습니다: %s", got[0].Path)
	}
	if got[0].Meta.Method.Name != "Handle" {
		t.Fatalf("등록된 메서드가 잘못되었습니다: %s", got[0].Meta.Method.Name)
	}

	got[0].Path = "/mutated"
	again := registry.Registrations()
	if again[0].Path != "/ws/echo" {
		t.Fatalf("Registrations는 복사본을 반환해야 합니다: %s", again[0].Path)
	}
}

func TestRegistry_RegisterPanicsOnInvalidInput(t *testing.T) {
	registry := NewRegistry()

	assertPanics(t, func() {
		registry.Register("", (*registryTestController).Handle)
	})

	assertPanics(t, func() {
		registry.Register("/ws/echo", nil)
	})
}

func assertPanics(t *testing.T, fn func()) {
	t.Helper()

	defer func() {
		if recover() == nil {
			t.Fatal("panic이 발생해야 합니다")
		}
	}()

	fn()
}
