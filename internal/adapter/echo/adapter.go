package echo

import (
	"fmt"

	"github.com/NARUBROWN/spine/internal/pipeline"
	"github.com/labstack/echo/v4"
)

// Adapter는 Echo 요청을 Spine 실행 모델로 연결합니다.
type Adapter struct {
	pipeline *pipeline.Pipeline
}

func NewAdapter(pipeline *pipeline.Pipeline) *Adapter {
	return &Adapter{
		pipeline: pipeline,
	}
}

// Mount는 Echo 인스턴스에 Spine 핸들러를 연결합니다.
func (a *Adapter) Mount(e *echo.Echo) {
	e.Any("/*", func(c echo.Context) error {
		ctx := NewContext(c)
		if err := a.pipeline.Execute(ctx); err != nil {
			fmt.Println("PIPELINE ERROR:", err)
			return err
		}
		return nil
	})
}
