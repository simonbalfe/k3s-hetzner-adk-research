.PHONY: check build image secrets plan apply bootstrap platform deploy

check:
	go vet ./...
	go build ./...

build:
	go build ./cmd/api ./cmd/worker

image:
	docker build -t k3s-hetzner-adk-research:dev .

secrets:
	./scripts/init-secrets.sh

plan:
	./scripts/infra-plan.sh

apply:
	./scripts/infra-apply.sh

bootstrap:
	./scripts/bootstrap-cluster.sh

platform:
	./scripts/install-platform.sh

deploy:
	./scripts/deploy.sh
