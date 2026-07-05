.PHONY: up down build restart logs ps mysql-shell clean

# Start all services in the background
up:
	docker compose up -d

# Stop all services
down:
	docker compose down

# Rebuild all docker images
build:
	docker compose build

# Restart all services
restart:
	docker compose restart

# View logs for all services in real-time
logs:
	docker compose logs -f

# Show status of all services
ps:
	docker compose ps

# Access MySQL shell directly inside the container
mysql-shell:
	docker compose exec mysql mysql -uappuser -papppass garmin_weight_sync

# Stop services and remove all volumes (clean start)
clean:
	docker compose down -v
