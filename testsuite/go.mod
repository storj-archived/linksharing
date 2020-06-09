module storj.io/linksharing/testsuite

go 1.13

replace storj.io/linksharing => ../

require (
	github.com/stretchr/testify v1.5.1
	go.uber.org/zap v1.15.0
	google.golang.org/grpc v1.28.0 // indirect
	storj.io/common v0.0.0-20200611114417-9a3d012fdb62
	storj.io/linksharing v0.0.0-00010101000000-000000000000
	storj.io/storj v0.12.1-0.20200612064134-410d897840a0
)
