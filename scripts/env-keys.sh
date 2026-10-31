#!/bin/sh
# env-keys.sh — prepare .env for docker compose (called by `make init` / `make regen-keys`).
#
#   scripts/env-keys.sh init    create .env from .env.example if missing and fill
#                               JWT_SECRET, DATA_ENCRYPTION_KEY and RSA_PRIVATE_KEY
#                               when they are empty or placeholders (never overwrites)
#   scripts/env-keys.sh regen   regenerate all three keys (asks for confirmation;
#                               existing encrypted data becomes unreadable)
set -eu

cd "$(dirname "$0")/.."
ENV_FILE=.env

configured() {
	value=$(grep "^$1=" "$ENV_FILE" 2>/dev/null | head -1 | cut -d= -f2- | tr -d "\"'")
	case "$value" in
	"" | *your-* | *YOUR_* | *change-this* | *CHANGE_THIS* | *example* | *EXAMPLE*) return 1 ;;
	esac
	return 0
}

# set_var NAME VALUE replaces or appends NAME=VALUE. The value is passed through
# ENVIRON so awk does not interpret its backslash sequences.
set_var() {
	tmp=$(mktemp)
	NAME="$1" VALUE="$2" awk '
		index($0, ENVIRON["NAME"] "=") == 1 { print ENVIRON["NAME"] "=" ENVIRON["VALUE"]; done = 1; next }
		{ print }
		END { if (!done) print ENVIRON["NAME"] "=" ENVIRON["VALUE"] }
	' "$ENV_FILE" >"$tmp"
	cat "$tmp" >"$ENV_FILE"
	rm -f "$tmp"
}

gen_jwt() { set_var JWT_SECRET "$(openssl rand -base64 32)"; }
gen_data_key() { set_var DATA_ENCRYPTION_KEY "$(openssl rand -base64 32)"; }
# The PEM is stored on one line with literal \n, which the .env loader expands.
gen_rsa() { set_var RSA_PRIVATE_KEY "\"$(openssl genrsa 2048 2>/dev/null | awk '{printf "%s\\n", $0}')\""; }

ensure_env() {
	if [ ! -f "$ENV_FILE" ]; then
		cp .env.example "$ENV_FILE"
		echo "Created $ENV_FILE from .env.example"
	fi
}

case "${1:-}" in
init)
	ensure_env
	configured JWT_SECRET || { gen_jwt && echo "Generated JWT_SECRET"; }
	configured DATA_ENCRYPTION_KEY || { gen_data_key && echo "Generated DATA_ENCRYPTION_KEY"; }
	configured RSA_PRIVATE_KEY || { gen_rsa && echo "Generated RSA_PRIVATE_KEY"; }
	chmod 600 "$ENV_FILE" 2>/dev/null || true
	echo "$ENV_FILE is ready"
	;;
regen)
	ensure_env
	echo "This regenerates JWT_SECRET, DATA_ENCRYPTION_KEY and RSA_PRIVATE_KEY."
	echo "Stored API keys encrypted with the old DATA_ENCRYPTION_KEY become unreadable."
	printf "Type yes to continue: "
	read -r answer
	[ "$answer" = yes ] || { echo "Cancelled"; exit 1; }
	gen_jwt
	gen_data_key
	gen_rsa
	chmod 600 "$ENV_FILE" 2>/dev/null || true
	echo "All keys regenerated"
	;;
*)
	echo "usage: $0 init|regen" >&2
	exit 2
	;;
esac
