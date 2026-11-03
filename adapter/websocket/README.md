# WebSocket 계약 호환 경로

`github.com/NARUBROWN/spine/adapter/websocket`은 [`contract/websocket`](../../contract/websocket/README.md)의 타입·계약 버전·오류를 그대로 제공하는 호환 별칭입니다. HTTP·이벤트와 같은 `adapter/` 아래에서 관리합니다.

루트의 `github.com/NARUBROWN/spine/wsadapter`를 사용하던 코드는 이 경로로 import를 바꿉니다. 새 구현은 정식 `github.com/NARUBROWN/spine/contract/websocket` 경로를 사용합니다.
