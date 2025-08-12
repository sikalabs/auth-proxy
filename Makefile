TAG = v0.1.4
IMAGE = sikalabs/auth-proxy
IMAGE_LATEST = ${IMAGE}:latest
IMAGE_TAGGED = ${IMAGE}:${TAG}
IMAGE_LATEST_GHCR = ghcr.io/${IMAGE}:latest
IMAGE_TAGGED_GHCR = ghcr.io/${IMAGE}:${TAG}

run:
	go run main.go

docker-all:
	@make docker-build
	@make docker-push
	@make docker-push-ghcr

docker-build:
	docker build --platform linux/amd64 -t ${IMAGE_LATEST} -t ${IMAGE_TAGGED} .

docker-push:
	docker push ${IMAGE_LATEST}
	docker push ${IMAGE_TAGGED}

docker-push-ghcr:
	docker tag ${IMAGE_LATEST} ${IMAGE_LATEST_GHCR}
	docker tag ${IMAGE_TAGGED} ${IMAGE_TAGGED_GHCR}
	docker push ${IMAGE_LATEST_GHCR}
	docker push ${IMAGE_TAGGED_GHCR}
