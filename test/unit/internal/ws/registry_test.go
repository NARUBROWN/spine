package ws

import "testing"

type registryTestController struct{}

func (c *registryTestController) Handle() {}

func TestRegistry_RegisterAndCopy(t *testing.T) {
	registry := NewRegistry()

	if err := registry.Register("/ws/echo", (*registryTestController).Handle); err != nil {
		t.Fatalf("등록 실패: %v", err)
	}

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

func TestRegistry_RegisterReturnsErrorOnInvalidInput(t *testing.T) {
	registry := NewRegistry()

	if err := registry.Register("", (*registryTestController).Handle); err == nil {
		t.Fatal("빈 path는 에러여야 합니다")
	}
	if err := registry.Register("/ws/echo", nil); err == nil {
		t.Fatal("nil handler는 에러여야 합니다")
	}
}
