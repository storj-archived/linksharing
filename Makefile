GO_VERSION ?= 1.15.6
GOOS ?= linux
GOARCH ?= amd64
GOPATH ?= $(shell go env GOPATH)
COMPOSE_PROJECT_NAME := ${TAG}-$(shell git rev-parse --abbrev-ref HEAD)
BRANCH_NAME ?= $(shell git rev-parse --abbrev-ref HEAD | sed "s!/!-!g")
ifeq (${BRANCH_NAME},master)
TAG    := $(shell git rev-parse --short HEAD)-go${GO_VERSION}
TRACKED_BRANCH := true
LATEST_TAG := latest
else
TAG    := $(shell git rev-parse --short HEAD)-${BRANCH_NAME}-go${GO_VERSION}
ifneq (,$(findstring release-,$(BRANCH_NAME)))
TRACKED_BRANCH := true
LATEST_TAG := ${BRANCH_NAME}-latest
endif
endif
CUSTOMTAG ?=

FILEEXT :=
ifeq (${GOOS},windows)
FILEEXT := .exe
endif

DOCKER_BUILD := docker build \
	--build-arg TAG=${TAG}

.DEFAULT_GOAL := help
.PHONY: help
help:
	@awk 'BEGIN { \
		FS = ":.*##"; \
		printf "\nUsage:\n  make \033[36m<target>\033[0m\n"\
	} \
	/^[a-zA-Z_-]+:.*?##/ { \
		printf "  \033[36m%-17s\033[0m %s\n", $$1, $$2 \
	} \
	/^##@/ { \
		printf "\n\033[1m%s\033[0m\n", substr($$0, 5) \
	} ' $(MAKEFILE_LIST)

##@ Dependencies

.PHONY: build-dev-deps
build-dev-deps: ## Install dependencies for builds
	go get golang.org/x/tools/cover
	go get github.com/josephspurrier/goversioninfo/cmd/goversioninfo

.PHONY: lint
lint: ## Analyze and find programs in source code
	@echo "Running ${@}"
	@golangci-lint run

.PHONY: goimports-fix
goimports-fix: ## Applies goimports to every go file (excluding vendored files)
	goimports -w -local storj.io $$(find . -type f -name '*.go' -not -path "*/vendor/*")

.PHONY: goimports-st
goimports-st: ## Applies goimports to every go file in `git status` (ignores untracked files)
	@git status --porcelain -uno|grep .go|grep -v "^D"|sed -E 's,\w+\s+(.+->\s+)?,,g'|xargs -I {} goimports -w -local storj.io {}

.PHONY: build-packages
build-packages: build-packages-normal build-packages-race ## Test docker images locally
build-packages-normal:
	go build -v ./...
build-packages-race:
	go build -v -race ./...

##@ Test

.PHONY: test
test: ## Run tests on source code (jenkins)
	go test -race -v -cover -coverprofile=.coverprofile ./...
	@echo done

##@ Build

.PHONY: images
images: linksharing-image ## Build linksharing Docker images
	echo Built version: ${TAG}

.PHONY: linksharing-image
linksharing-image: linksharing_linux_arm64 linksharing_linux_amd64 ## Build linksharing Docker image
	${DOCKER_BUILD} --pull=true -t storjlabs/linksharing:${TAG}-amd64 \
		-f Dockerfile .
	${DOCKER_BUILD} --pull=true -t storjlabs/linksharing:${TAG}-arm32v6 \
		--build-arg=GOARCH=arm --build-arg=DOCKER_ARCH=arm32v6 \
		-f Dockerfile .
	${DOCKER_BUILD} --pull=true -t storjlabs/linksharing:${TAG}-aarch64 \
		--build-arg=GOARCH=arm64 --build-arg=DOCKER_ARCH=aarch64 \
		-f Dockerfile .

.PHONY: binary
binary: CUSTOMTAG = -${GOOS}-${GOARCH}
binary:
	@if [ -z "${COMPONENT}" ]; then echo "Try one of the following targets instead:" \
		&& for b in binaries ${BINARIES}; do echo "- $$b"; done && exit 1; fi
	mkdir -p release/${TAG}
	mkdir -p /tmp/go-cache /tmp/go-pkg
	rm -f private/cmd/resource.syso
	if [ "${GOARCH}" = "amd64" ]; then sixtyfour="-64"; fi; \
	[ "${GOOS}" = "windows" ] && [ "${GOARCH}" = "amd64" ] && goversioninfo $$sixtyfour -o private/cmd/resource.syso \
	-original-name ${COMPONENT}_${GOOS}_${GOARCH}${FILEEXT} \
	-description "${COMPONENT} program for Storj" \
        -product-ver-major "$(shell git describe --tags --exact-match --match "v[0-9]*\.[0-9]*\.[0-9]*" | awk -F'.' 'BEGIN {v=0} {gsub("v", "", $$0); v=$$1} END {print v}' )" \
                -ver-major "$(shell git describe --tags --exact-match --match "v[0-9]*\.[0-9]*\.[0-9]*" | awk -F'.' 'BEGIN {v=0} {gsub("v", "", $$0); v=$$1} END {print v}' )" \
        -product-ver-minor "$(shell git describe --tags --exact-match --match "v[0-9]*\.[0-9]*\.[0-9]*" | awk -F'.' 'BEGIN {v=0} {v=$$2} END {print v}')" \
                -ver-minor "$(shell git describe --tags --exact-match --match "v[0-9]*\.[0-9]*\.[0-9]*" | awk -F'.' 'BEGIN {v=0} {v=$$2} END {print v}')" \
        -product-ver-patch "$(shell git describe --tags --exact-match --match "v[0-9]*\.[0-9]*\.[0-9]*" | awk -F'.' 'BEGIN {v=0} {v=$$3} END {print v}' | awk -F'-' 'BEGIN {v=0} {v=$$1} END {print v}')" \
                -ver-patch "$(shell git describe --tags --exact-match --match "v[0-9]*\.[0-9]*\.[0-9]*" | awk -F'.' 'BEGIN {v=0} {v=$$3} END {print v}' | awk -F'-' 'BEGIN {v=0} {v=$$1} END {print v}')" \
        -product-version "$(shell git describe --tags --exact-match --match "v[0-9]*\.[0-9]*\.[0-9]*" | awk -F'-' 'BEGIN {v=0} {v=$$1} END {print v}' || echo "dev" )" \
        -special-build "$(shell git describe --tags --exact-match --match "v[0-9]*\.[0-9]*\.[0-9]*" | awk -F'-' 'BEGIN {v=0} {v=$$2} END {print v}' )" \
	resources/versioninfo.json || echo "goversioninfo is not installed, metadata will not be created"
	docker run --rm -i -v "${PWD}":/go/src/storj.io/storj -e GO111MODULE=on \
	-e GOOS=${GOOS} -e GOARCH=${GOARCH} -e GOARM=6 -e CGO_ENABLED=1 \
	-v /tmp/go-cache:/tmp/.cache/go-build -v /tmp/go-pkg:/go/pkg \
	-w /go/src/storj.io/storj -e GOPROXY -u $(shell id -u):$(shell id -g) storjlabs/golang:${GO_VERSION} \
	scripts/release.sh build $(EXTRA_ARGS) -o release/${TAG}/$(COMPONENT)_${GOOS}_${GOARCH}${FILEEXT} ./cmd/linksharing
	chmod 755 release/${TAG}/$(COMPONENT)_${GOOS}_${GOARCH}${FILEEXT}
	[ "${FILEEXT}" = ".exe" ] && storj-sign release/${TAG}/$(COMPONENT)_${GOOS}_${GOARCH}${FILEEXT} || echo "Skipping signing"
	rm -f release/${TAG}/${COMPONENT}_${GOOS}_${GOARCH}.zip

.PHONY: binary-check
binary-check:
	@if [ -f release/${TAG}/${COMPONENT}_${GOOS}_${GOARCH} ] || [ -f release/${TAG}/${COMPONENT}_${GOOS}_${GOARCH}.exe ]; \
	then \
		echo "release/${TAG}/${COMPONENT}_${GOOS}_${GOARCH} exists"; \
	else \
		echo "Making ${COMPONENT}"; \
		$(MAKE) binary; \
	fi

.PHONY: linksharing_%
linksharing_%:
	$(MAKE) binary-check COMPONENT=linksharing GOARCH=$(word 3, $(subst _, ,$@)) GOOS=$(word 2, $(subst _, ,$@))

COMPONENTLIST := linksharing
OSARCHLIST    := darwin_amd64 linux_amd64 linux_arm linux_arm64 windows_amd64 freebsd_amd64
BINARIES      := $(foreach C,$(COMPONENTLIST),$(foreach O,$(OSARCHLIST),$C_$O))
.PHONY: binaries
binaries: ${BINARIES} ## Build linksharing binaries (jenkins)

##@ Deploy

.PHONY: push-images
push-images: ## Push Docker images to Docker Hub (jenkins)
	# images have to be pushed before a manifest can be created
	for c in linksharing; do \
		docker push storjlabs/$$c:${TAG}-amd64 \
		&& docker push storjlabs/$$c:${TAG}-arm32v6 \
		&& docker push storjlabs/$$c:${TAG}-aarch64 \
		&& for t in ${TAG} ${LATEST_TAG}; do \
			docker manifest create storjlabs/$$c:$$t \
			storjlabs/$$c:${TAG}-amd64 \
			storjlabs/$$c:${TAG}-arm32v6 \
			storjlabs/$$c:${TAG}-aarch64 \
			&& docker manifest annotate storjlabs/$$c:$$t storjlabs/$$c:${TAG}-amd64 --os linux --arch amd64 \
			&& docker manifest annotate storjlabs/$$c:$$t storjlabs/$$c:${TAG}-arm32v6 --os linux --arch arm --variant v6 \
			&& docker manifest annotate storjlabs/$$c:$$t storjlabs/$$c:${TAG}-aarch64 --os linux --arch arm64 \
			&& docker manifest push --purge storjlabs/$$c:$$t \
		; done \
	; done

.PHONY: binaries-upload
binaries-upload: ## Upload binaries to Google Storage (jenkins)
	cd "release/${TAG}"; for f in *; do \
		c="$${f%%_*}" \
		&& if [ "$${f##*.}" != "$${f}" ]; then \
			ln -s "$${f}" "$${f%%_*}.$${f##*.}" \
			&& zip "$${f}.zip" "$${f%%_*}.$${f##*.}" \
			&& rm "$${f%%_*}.$${f##*.}" \
		; else \
			ln -sf "$${f}" "$${f%%_*}" \
			&& zip "$${f}.zip" "$${f%%_*}" \
			&& rm "$${f%%_*}" \
		; fi \
	; done
	cd "release/${TAG}"; gsutil -m cp -r *.zip "gs://storj-v3-alpha-builds/${TAG}/"

##@ Clean

.PHONY: clean
clean: binaries-clean clean-images ## Clean local release binaries, and local Docker images

.PHONY: binaries-clean
binaries-clean: ## Remove all local release binaries (jenkins)
	rm -rf release

.PHONY: clean-images
clean-images:
        -docker rmi storjlabs/linksharing:${TAG}

.PHONY: bump-dependencies
bump-dependencies:
	go get storj.io/common@master storj.io/private@master storj.io/uplink@master
	go mod tidy
	cd testsuite;\
		go get storj.io/common@master storj.io/storj@master;\
		go mod tidy
