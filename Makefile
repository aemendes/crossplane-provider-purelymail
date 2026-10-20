IMAGE ?= ghcr.io/aemendes/provider-purelymail:latest
PKG_IMAGE ?= ghcr.io/aemendes/provider-purelymail-pkg:latest
PLATFORM ?= linux/amd64
CONTROLLER_GEN ?= .bin/controller-gen

.PHONY: controller-gen
controller-gen:
	GOBIN=$(CURDIR)/.bin go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5

.PHONY: generate
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="" paths="./..."
	$(CONTROLLER_GEN) crd paths="./..." output:crd:artifacts:config=package/crds

.PHONY: build
build: generate
	go build ./...

.PHONY: test
test: generate
	go test ./...

.PHONY: docker-build
docker-build:
	docker buildx build --platform $(PLATFORM) -t $(IMAGE) --load .

.PHONY: docker-push
docker-push: docker-build
	docker push $(IMAGE)

.PHONY: xpkg-build
xpkg-build: docker-build
	crossplane xpkg build --package-root=package --embed-runtime-image=$(IMAGE) -o provider-purelymail.xpkg

.PHONY: xpkg-push
xpkg-push: xpkg-build
	crossplane xpkg push -f provider-purelymail.xpkg $(PKG_IMAGE)
