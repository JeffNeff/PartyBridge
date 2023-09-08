run: build
	docker build . --tag partybridge:dev
	docker-compose -f docker-compose-dev.yaml up

up:
	@docker compose up -d

down:
	@docker compose down

# run `sipper
sip:
	@cd sipper && go run .

image: 
	@docker build -t jeffthenaef/pb . 
	@docker push jeffthenaef/pb

build:
	cd cmd/partybridge && go mod tidy && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o partybridge main.go

debug: 
	@make up
	@docker logs teaparty-partybridge-1 --tail 50 -f

# Build fresh versions of the docker containers, start a local stack, and run `sipper` against it. 
test: 
	@make build
	@make up
	@make sip

attach:
	@docker logs teaparty-partybridge-1 --tail 50 -f

watch:
	@kubectl logs -l  app=partybridge -f

ko:
	@export KO_DOCKER_REPO=gcr.io/mineonlium && ko apply -f ko.yaml
