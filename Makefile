idl:
	./kitex_gen.sh

env:
	docker-compose up -d

build:
	cd cmd/api && go build -o api
	cd cmd/user && go build -o user
	cd cmd/video && go build -o video
	cd cmd/relation && go build -o relation
	cd cmd/interaction && go build -o interaction

api:
	cd cmd/api && ./api
users:
	cd cmd/user && ./user
videos:
	cd cmd/video && ./video
interactions:
	cd cmd/interaction && ./interaction
relations:
	cd cmd/relation && ./relation

go: env build

init-db:
	docker-compose exec -T mysql mysql -u root -p'TikTok@MySQL#2025!Secure' < config/mysql/init.sql

init-rec-db:
	docker-compose exec -T mysql mysql -u root -p'TikTok@MySQL#2025!Secure' < config/mysql/recommendation_init.sql

init-all-db: init-db init-rec-db

clean:
	-cd cmd/api && rm -f api
	-cd cmd/user && rm -f user
	-cd cmd/video && rm -f video
	-cd cmd/relation && rm -f relation
	-cd cmd/interaction && rm -f interaction
