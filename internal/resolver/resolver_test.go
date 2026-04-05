package resolver

import (
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"reflect"
	"testing"
	"time"

	"github.com/NARUBROWN/spine/core"
	eventpublish "github.com/NARUBROWN/spine/pkg/event/publish"
	"github.com/NARUBROWN/spine/pkg/header"
	pkgmultipart "github.com/NARUBROWN/spine/pkg/multipart"
	"github.com/NARUBROWN/spine/pkg/path"
	"github.com/NARUBROWN/spine/pkg/query"
)

type fakeHttpCtx struct {
	method        string
	path          string
	params        map[string]string
	queries       map[string][]string
	headers       map[string][]string
	pathKeys      []string
	store         map[string]any
	bindValue     any
	bindErr       error
	multipartForm *multipart.Form
	multipartErr  error
	bus           core.EventBus
}

func newFakeHttpCtx() *fakeHttpCtx {
	return &fakeHttpCtx{
		method:   "GET",
		path:     "/",
		params:   map[string]string{},
		queries:  map[string][]string{},
		headers:  map[string][]string{},
		pathKeys: []string{},
		store:    map[string]any{},
	}
}

// ExecutionContext
func (c *fakeHttpCtx) Context() context.Context  { return context.Background() }
func (c *fakeHttpCtx) EventBus() core.EventBus   { return c.bus }
func (c *fakeHttpCtx) Method() string            { return c.method }
func (c *fakeHttpCtx) Path() string              { return c.path }
func (c *fakeHttpCtx) Params() map[string]string { return c.params }
func (c *fakeHttpCtx) Header(name string) string {
	if vs, ok := c.headers[name]; ok && len(vs) > 0 {
		return vs[0]
	}
	return ""
}
func (c *fakeHttpCtx) PathKeys() []string           { return c.pathKeys }
func (c *fakeHttpCtx) Queries() map[string][]string { return c.queries }
func (c *fakeHttpCtx) Set(key string, value any)    { c.store[key] = value }
func (c *fakeHttpCtx) Get(key string) (any, bool)   { v, ok := c.store[key]; return v, ok }

// HttpRequestContext
func (c *fakeHttpCtx) Param(name string) string { return c.params[name] }
func (c *fakeHttpCtx) Query(name string) string {
	if vs, ok := c.queries[name]; ok && len(vs) > 0 {
		return vs[0]
	}
	return ""
}
func (c *fakeHttpCtx) Headers() map[string][]string { return c.headers }
func (c *fakeHttpCtx) Bind(out any) error {
	if c.bindErr != nil {
		return c.bindErr
	}
	if c.bindValue != nil {
		reflect.ValueOf(out).Elem().Set(reflect.ValueOf(c.bindValue))
	}
	return nil
}
func (c *fakeHttpCtx) MultipartForm() (*multipart.Form, error) {
	return c.multipartForm, c.multipartErr
}

type fakeEventBus struct {
	published []eventpublish.DomainEvent
}

func (b *fakeEventBus) Publish(events ...eventpublish.DomainEvent) {
	b.published = append(b.published, events...)
}
func (b *fakeEventBus) Drain() []eventpublish.DomainEvent { return nil }

type sampleEvent struct {
	name string
	at   time.Time
}

func (e sampleEvent) Name() string          { return e.name }
func (e sampleEvent) OccurredAt() time.Time { return e.at }

func TestPathIntResolver_Success(t *testing.T) {
	r := &PathIntResolver{}
	pm := ParameterMeta{Type: reflect.TypeFor[path.Int](), PathKey: "id"}
	ctx := newFakeHttpCtx()
	ctx.params["id"] = "42"

	val, err := r.Resolve(ctx, pm)
	if err != nil {
		t.Fatalf("PathIntResolver 실패: %v", err)
	}
	if val.(path.Int).Value != 42 {
		t.Fatalf("값이 잘못되었습니다: %v", val)
	}
}

func TestPathIntResolver_InvalidBool(t *testing.T) {
	r := &PathBooleanResolver{}
	pm := ParameterMeta{Type: reflect.TypeFor[path.Boolean](), PathKey: "flag"}
	ctx := newFakeHttpCtx()
	ctx.params["flag"] = "maybe"

	_, err := r.Resolve(ctx, pm)
	if err == nil {
		t.Fatal("잘못된 불리언은 에러여야 합니다")
	}
}

func TestPathBooleanResolver_TrueVariants(t *testing.T) {
	r := &PathBooleanResolver{}
	pm := ParameterMeta{Type: reflect.TypeFor[path.Boolean](), PathKey: "flag"}
	ctx := newFakeHttpCtx()
	ctx.params["flag"] = "YeS"

	val, err := r.Resolve(ctx, pm)
	if err != nil {
		t.Fatalf("불리언 파싱 실패: %v", err)
	}
	if !val.(path.Boolean).Value {
		t.Fatalf("불리언 값이 true 여야 합니다: %v", val)
	}
}

func TestPaginationResolver_Defaults(t *testing.T) {
	r := &PaginationResolver{}
	pm := ParameterMeta{Type: reflect.TypeFor[query.Pagination]()}
	ctx := newFakeHttpCtx()

	val, err := r.Resolve(ctx, pm)
	if err != nil {
		t.Fatalf("PaginationResolver 실패: %v", err)
	}
	p := val.(query.Pagination)
	if p.Page != 1 || p.Size != 20 {
		t.Fatalf("기본값이 잘못되었습니다: %+v", p)
	}
}

func TestPaginationResolver_ParseValues(t *testing.T) {
	r := &PaginationResolver{}
	pm := ParameterMeta{Type: reflect.TypeFor[query.Pagination]()}
	ctx := newFakeHttpCtx()
	ctx.queries["page"] = []string{"3"}
	ctx.queries["size"] = []string{"50"}

	val, _ := r.Resolve(ctx, pm)
	p := val.(query.Pagination)
	if p.Page != 3 || p.Size != 50 {
		t.Fatalf("쿼리 파싱 결과가 잘못되었습니다: %+v", p)
	}
}

type dtoSample struct {
	Name string
	Age  int
}

type formTagged struct {
	Name string `form:"name"`
}

func TestDTOResolver_SupportsAndResolve(t *testing.T) {
	r := &DTOResolver{}
	pm := ParameterMeta{Type: reflect.TypeOf(&dtoSample{})}
	ctx := newFakeHttpCtx()
	ctx.bindValue = dtoSample{Name: "abc", Age: 10}

	if !r.Supports(pm) {
		t.Fatal("DTOResolver가 포인터 struct DTO를 지원해야 합니다")
	}

	val, err := r.Resolve(ctx, pm)
	if err != nil {
		t.Fatalf("DTOResolver Resolve 실패: %v", err)
	}
	dto := val.(*dtoSample)
	if dto.Name != "abc" || dto.Age != 10 {
		t.Fatalf("바인딩 결과가 잘못되었습니다: %+v", dto)
	}
}

func TestDTOResolver_RejectsFormTaggedStruct(t *testing.T) {
	r := &DTOResolver{}
	pm := ParameterMeta{Type: reflect.TypeOf(&formTagged{})}
	if r.Supports(pm) {
		t.Fatal("form 태그가 있는 struct는 DTOResolver가 지원하지 않아야 합니다")
	}
}

func TestDTOResolver_BindErrorPropagates(t *testing.T) {
	r := &DTOResolver{}
	pm := ParameterMeta{Type: reflect.TypeOf(&dtoSample{})}
	ctx := newFakeHttpCtx()
	ctx.bindErr = errors.New("bind fail")

	_, err := r.Resolve(ctx, pm)
	if err == nil {
		t.Fatal("Bind 에러가 전파되어야 합니다")
	}
}

func TestFormDTOResolver_SupportsAndResolve(t *testing.T) {
	r := &FormDTOResolver{}
	pm := ParameterMeta{Type: reflect.TypeOf(&formTagged{})}
	ctx := newFakeHttpCtx()
	ctx.bindValue = formTagged{Name: "kim"}

	if !r.Supports(pm) {
		t.Fatal("form 태그가 있는 struct는 FormDTOResolver가 지원해야 합니다")
	}

	val, err := r.Resolve(ctx, pm)
	if err != nil {
		t.Fatalf("FormDTOResolver Resolve 실패: %v", err)
	}
	dto := val.(*formTagged)
	if dto.Name != "kim" {
		t.Fatalf("폼 DTO 값이 잘못되었습니다: %+v", dto)
	}
}

func TestHeaderResolver_Resolve(t *testing.T) {
	r := &HeaderResolver{}
	pm := ParameterMeta{Type: reflect.TypeFor[header.Values]()}
	ctx := newFakeHttpCtx()
	ctx.headers["X-Trace-Id"] = []string{"abc"}

	val, err := r.Resolve(ctx, pm)
	if err != nil {
		t.Fatalf("HeaderResolver Resolve 실패: %v", err)
	}

	headers := val.(header.Values)
	if !headers.Has("X-Trace-Id") || headers.Get("X-Trace-Id") != "abc" {
		t.Fatalf("헤더 조회 결과가 잘못되었습니다")
	}
}

func TestQueryValuesResolver_Resolve(t *testing.T) {
	r := &QueryValuesResolver{}
	pm := ParameterMeta{Type: reflect.TypeFor[query.Values]()}
	ctx := newFakeHttpCtx()
	ctx.queries["page"] = []string{"3"}
	ctx.queries["enabled"] = []string{"yes"}

	val, err := r.Resolve(ctx, pm)
	if err != nil {
		t.Fatalf("QueryValuesResolver Resolve 실패: %v", err)
	}

	values := val.(query.Values)
	if values.Int("page", 0) != 3 || !values.GetBoolByKey("enabled", false) {
		t.Fatalf("쿼리 조회 결과가 잘못되었습니다")
	}
}

func TestControllerContextResolver_Resolve(t *testing.T) {
	r := &ControllerContextResolver{}
	pm := ParameterMeta{Type: reflect.TypeOf((*core.ControllerContext)(nil)).Elem()}
	ctx := newFakeHttpCtx()
	ctx.store["userID"] = 7

	if !r.Supports(pm) {
		t.Fatal("ControllerContextResolver는 ControllerContext를 지원해야 합니다")
	}

	val, err := r.Resolve(ctx, pm)
	if err != nil {
		t.Fatalf("ControllerContextResolver Resolve 실패: %v", err)
	}

	controllerCtx := val.(core.ControllerContext)
	got, ok := controllerCtx.Get("userID")
	if !ok || got.(int) != 7 {
		t.Fatalf("ControllerContext가 store를 읽지 못했습니다: %v %v", got, ok)
	}
}

func TestStdContextResolver_InjectsPublisher(t *testing.T) {
	r := &StdContextResolver{}
	pm := ParameterMeta{Type: reflect.TypeFor[context.Context]()}
	ctx := newFakeHttpCtx()
	bus := &fakeEventBus{}
	ctx.bus = bus

	val, err := r.Resolve(ctx, pm)
	if err != nil {
		t.Fatalf("StdContextResolver Resolve 실패: %v", err)
	}

	resolvedCtx := val.(context.Context)
	eventpublish.Event(resolvedCtx, sampleEvent{name: "order.created", at: time.Now()})

	if len(bus.published) != 1 || bus.published[0].Name() != "order.created" {
		t.Fatalf("publisher가 context에 주입되지 않았습니다: %+v", bus.published)
	}
}

func TestUploadedFilesResolver_Resolve(t *testing.T) {
	r := &UploadedFilesResolver{}
	pm := ParameterMeta{Type: reflect.TypeFor[pkgmultipart.UploadedFiles]()}
	ctx := newFakeHttpCtx()
	ctx.multipartForm = &multipart.Form{
		File: map[string][]*multipart.FileHeader{
			"avatar": {
				{
					Filename: "avatar.png",
					Header: textproto.MIMEHeader{
						"Content-Type": []string{"image/png"},
					},
					Size: 10,
				},
			},
		},
	}

	val, err := r.Resolve(ctx, pm)
	if err != nil {
		t.Fatalf("UploadedFilesResolver Resolve 실패: %v", err)
	}

	files := val.(pkgmultipart.UploadedFiles)
	if len(files.Files) != 1 {
		t.Fatalf("파일 개수가 잘못되었습니다: %d", len(files.Files))
	}
	if files.Files[0].FieldName != "avatar" || files.Files[0].Filename != "avatar.png" || files.Files[0].ContentType != "image/png" || files.Files[0].Size != 10 {
		t.Fatalf("파일 메타데이터가 잘못되었습니다: %+v", files.Files[0])
	}
}

func TestUploadedFilesResolver_PropagatesMultipartError(t *testing.T) {
	r := &UploadedFilesResolver{}
	pm := ParameterMeta{Type: reflect.TypeFor[pkgmultipart.UploadedFiles]()}
	ctx := newFakeHttpCtx()
	ctx.multipartErr = http.ErrMissingFile

	if _, err := r.Resolve(ctx, pm); !errors.Is(err, http.ErrMissingFile) {
		t.Fatalf("Multipart 에러가 전파되어야 합니다: %v", err)
	}
}
