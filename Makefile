include app.env
export

network:
	docker network create assignme

postgres:
	docker run --name postgres --network assignme -p 5432:5432 -e POSTGRES_USER=root -e POSTGRES_PASSWORD=auxano3454 -d postgres:14-alpine

mysql:
	docker run --name mysql8 -p 3306:3306  -e MYSQL_ROOT_PASSWORD=secret -d mysql:8

createdb:
	docker exec -it postgres createdb --username=root --owner=root assignme

dropdb:
	docker exec -it postgres dropdb assignme

migrateup:
	migrate -path db/migration -database "$(DB_SOURCE)" -verbose up

migrateup1:
	migrate -path db/migration -database "$(DB_SOURCE)" -verbose up 1

migratedown:
	migrate -path db/migration -database "$(DB_SOURCE)" -verbose down

migratedown1:
	migrate -path db/migration -database "$(DB_SOURCE)" -verbose down 1

new_migration:
	migrate create -ext sql -dir db/migration -seq $(name)

db_docs:
	dbdocs build doc/db.dbml

db_schema:
	dbml2sql --postgres -o doc/schema.sql doc/db.dbml

redis:
	docker run --name redis --network assignme -p 6379:6379 -e REDIS_PASSWORD=auxano3454 -d redis:7-alpine redis-server --requirepass auxano3454

sqlc:
	sqlc generate

test:
	go test -v -cover -short ./...

server:
	go run main.go

mock:
	mockgen -package mockdb -destination db/mock/store.go github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc Store

zookeeper:
	docker run --name zookeeper --network assignme \
		-p 2181:2181 \
		-e ZOOKEEPER_CLIENT_PORT=2181 \
		-e ZOOKEEPER_TICK_TIME=2000 \
		-d confluentinc/cp-zookeeper:latest

kafka:
	docker run --name kafka --network assignme \
		-p 29092:29092 \
		-e KAFKA_BROKER_ID=1 \
		-e KAFKA_ZOOKEEPER_CONNECT=zookeeper:2181 \
		-e KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://kafka:9092,PLAINTEXT_HOST://localhost:29092 \
		-e KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=PLAINTEXT:PLAINTEXT,PLAINTEXT_HOST:PLAINTEXT \
		-e KAFKA_INTER_BROKER_LISTENER_NAME=PLAINTEXT \
		-e KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=1 \
		-d confluentinc/cp-kafka:latest

test:
	go test -v -cover -short ./...

.PHONY: network postgres createdb dropdb migrateup migratedown migrateup1 migratedown1 new_migration db_docs db_schema sqlc test server mock zookeeper kafka test