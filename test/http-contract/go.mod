module github.com/NARUBROWN/spine/test/http-contract

go 1.26.0

require (
	github.com/NARUBROWN/spine v0.7.0-preview.2
	github.com/valyala/fasthttp v1.77.0
)

require (
	github.com/klauspost/compress v1.20.3 // indirect
	github.com/molecule-man/go-brrr v1.3.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
)

replace github.com/NARUBROWN/spine => ../..
