#!/usr/bin/env bash
# Backs up the database, or restores a backup into a new database. The PostgreSQL in the
# vergex-postgres container does the work, so nothing needs installing on the host.
#
#   scripts/backup.sh backup [dir]         writes <dir>/vergex-<time>.sql.gz (default backups/)
#   scripts/backup.sh restore <file> <db>  restores into <db>, which must not exist yet
set -euo pipefail

cd "$(dirname "$0")/.."

env_value() {
	grep -E "^$1=" .env 2>/dev/null | tail -1 | cut -d= -f2- | tr -d "\"' " || true
}

container=vergex-postgres
user=$(env_value DB_USER)
user=${user:-vergex}
database=$(env_value DB_NAME)
database=${database:-vergex}

if ! docker exec "$container" pg_isready -q -U "$user"; then
	echo "The database is not running. Start it with make up or make run-postgresql." >&2
	exit 1
fi

backup() {
	local dir=${1:-backups}
	local file
	file="$dir/vergex-$(date +%Y%m%d-%H%M%S).sql.gz"
	# It holds encrypted keys and everything traded, so only its owner reads it.
	umask 077
	mkdir -p "$dir"
	if ! docker exec "$container" pg_dump -U "$user" --no-owner "$database" | gzip >"$file"; then
		rm -f "$file"
		echo "The backup failed." >&2
		exit 1
	fi
	echo "Backed up to $file ($(du -h "$file" | cut -f1)). Keep it with .env: the keys in it need DATA_ENCRYPTION_KEY."
}

restore() {
	local file=${1:-} target=${2:-}
	if [[ -z $file || -z $target ]]; then
		echo "Usage: make restore FILE=<backup.sql.gz> DB=<new database name>" >&2
		exit 1
	fi
	if [[ ! -f $file ]]; then
		echo "$file does not exist." >&2
		exit 1
	fi
	if [[ ! $target =~ ^[a-zA-Z_][a-zA-Z0-9_]*$ ]]; then
		echo "$target is not a usable database name." >&2
		exit 1
	fi
	exists=$(docker exec "$container" psql -U "$user" -d postgres -Atc "SELECT 1 FROM pg_database WHERE datname = '$target'")
	if [[ -n $exists ]]; then
		echo "Database $target already exists. A restore only goes into a new one, so nothing is overwritten." >&2
		exit 1
	fi
	docker exec "$container" createdb -U "$user" "$target"
	if ! gunzip -c "$file" | docker exec -i "$container" psql -q -v ON_ERROR_STOP=1 -U "$user" -d "$target" >/dev/null; then
		docker exec "$container" dropdb -U "$user" "$target"
		echo "The restore failed, and $target was removed again." >&2
		exit 1
	fi
	echo "Restored into $target. To run on it, stop the backend, set DB_NAME=$target in .env and start again."
}

case "${1:-}" in
backup) backup "${2:-}" ;;
restore) restore "${2:-}" "${3:-}" ;;
*)
	echo "Usage: $0 backup [dir] | restore <file> <database>" >&2
	exit 1
	;;
esac
