REPO := jjo/kube-gitlab-authn
# My dockerhub user is 'xjjo'
IMAGE_NAME := x$(REPO)
GO_SRC_PATH := /go/src/github.com/$(REPO)
PORT := 3000

# docker-run defaults
GITLAB_API_ENDPOINT=https://gitlab.com/api/v4/
GITLAB_GROUP_RE=.+


ifeq (1,${WITH_DOCKER})
DOCKER_RUN := docker run --rm -i \
	-v `pwd`:$(GO_SRC_PATH) \
	-w $(GO_SRC_PATH)
GO_RUN := $(DOCKER_RUN) golang:1.13-alpine
endif

.PHONY: build
build:
	$(GO_RUN) go build -o _output/main main.go

.PHONY: vendor
vendor:
	$(GLIDE_RUN) glide install

.PHONY: clean
clean:
	rm -rf _output

.PHONY: docker-build
docker-build:
	#WITH_DOCKER=1 make build
	docker build -t $(IMAGE_NAME) .

.PHONY: docker-run
docker-run:
	docker run -it --rm -e GITLAB_API_ENDPOINT=$(GITLAB_API_ENDPOINT) -e GITLAB_GROUP_RE=$(GITLAB_GROUP_RE) -p $(PORT):3000 $(IMAGE_NAME)
