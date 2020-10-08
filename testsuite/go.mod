module storj.io/linksharing/testsuite

go 1.13

replace storj.io/linksharing => ../

require (
	github.com/stretchr/testify v1.6.1
	go.uber.org/zap v1.15.0
	google.golang.org/grpc v1.28.0 // indirect
	storj.io/common v0.0.0-20200903093850-858d8a4eccde
	storj.io/linksharing v0.0.0-00010101000000-000000000000
	storj.io/storj v0.12.1-0.20200904110948-8649a0055710
)
