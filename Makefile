# umoci: Umoci Modifies Open Containers' Images
# Copyright (C) 2016 SUSE LLC.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#   http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

GO ?= go

# Set up the ... lovely ... GOPATH hacks.
PROJECT := github.com/cyphar/umoci

# We use Docker because Go is just horrific to deal with.
UMOCI_IMAGE := umoci_dev

# Version information.
VERSION := $(shell cat ./VERSION)
COMMIT_NO := $(shell git rev-parse HEAD 2> /dev/null || true)
COMMIT := $(if $(shell git status --porcelain --untracked-files=no),"${COMMIT_NO}-dirty","${COMMIT_NO}")

.DEFAULT: umoci

GO_SRC =  $(shell find . -name \*.go)
umoci: $(GO_SRC)
	$(GO) build -i -ldflags "-X main.gitCommit=${COMMIT} -X main.version=${VERSION}" -tags "$(BUILDTAGS)" -o umoci $(PROJECT)/cmd/umoci

.PHONY: update-deps
update-deps:
	hack/vendor.sh

.PHONY: clean
clean:
	rm -f umoci

validate: umociimage
	docker run --rm -it -v $(PWD):/go/src/$(PROJECT) $(UMOCI_IMAGE) make local-validate

EPOCH_COMMIT ?= 97ecdbd53dcb72b7a0d62196df281f131dc9eb2f
.PHONY: local-validate
local-validate:
	test -z "$$(gofmt -s -l . | grep -v '^vendor/' | grep -v '^third_party/' | tee /dev/stderr)"
	out="$$(golint $(PROJECT)/... | grep -v '/vendor/' | grep -v '/third_party/' | grep -vE 'system/utils_linux.*ALL_CAPS|system/mknod_linux.*underscores')"; \
	if [ -n "$$out" ]; then \
		echo "$$out"; \
		exit 1; \
	fi
	go vet $(shell go list $(PROJECT)/... | grep -v /vendor/ | grep -v /third_party/)
	#@echo "git-validation"
	#@git-validation -v -run DCO,short-subject,dangling-whitespace $(EPOCH_COMMIT)..HEAD

# Used for tests.
DOCKER_IMAGE :=opensuse/amd64:tumbleweed

.PHONY: umociimage
umociimage:
	docker build -t $(UMOCI_IMAGE) --build-arg DOCKER_IMAGE=$(DOCKER_IMAGE) .

.PHONY: test-unit
test-unit: umociimage
	docker run --rm -it -v $(PWD):/go/src/$(PROJECT) $(UMOCI_IMAGE) make local-test-unit

.PHONY: local-test-unit
local-test-unit: umoci
	go test -v $(PROJECT)/...

.PHONY: test-integration
test-integration: umociimage
	docker run --rm -it -v $(PWD):/go/src/$(PROJECT) $(UMOCI_IMAGE) make local-test-integration

.PHONY: local-test-integration
local-test-integration: umoci
	bats -t test/*.bats

.PHONY: ci
ci: umoci validate test-unit test-integration