module storj.io/linksharing/testsuite

go 1.13

replace storj.io/linksharing => ../

require (
	github.com/onsi/ginkgo v1.14.0 // indirect
	github.com/stretchr/testify v1.6.1
	go.uber.org/zap v1.16.0
	golang.org/x/net v0.0.0-20200707034311-ab3426394381 // indirect
	storj.io/common v0.0.0-20210104180112-e8500e1c37a0
	storj.io/linksharing v0.0.0-00010101000000-000000000000
	storj.io/storj v0.12.1-0.20210105155403-f925d99d3955
)
