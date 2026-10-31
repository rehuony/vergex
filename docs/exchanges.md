# Exchange API keys

vergex trades **Binance USDT-M futures** and **OKX USDT perpetual swaps**. Add accounts in **Settings → Exchanges → Add account**. The **Verify** button on an account checks the key against the exchange.

Rules for both exchanges:

- Enable reading and futures trading. **Never enable withdrawals.**
- Restrict the key to your server's IP. Settings → Exchanges shows it, and `GET /api/server-ip` returns it.
- Prefer a dedicated sub-account to isolate risk. Some sub-accounts have lower leverage limits.
- Exchange testnets are not supported, so try new setups with a small balance.

## Binance

1. Go to **Account → API Management → Create API** and choose a system-generated key.
2. Enable **Reading** and **Futures**.
3. Under **Edit restrictions**, select **Restrict access to trusted IPs only** and add the server IP.
4. Copy the API key and secret. The secret is shown only once.

Autopilot needs **Hedge Mode**. vergex sets it when it connects, but Binance refuses the change while you have open positions or orders. If you see `code=-4061`, close them and switch manually: Futures → Preferences → Position Mode → Hedge Mode.

## OKX

1. Go to **Profile → API → Create API key** with the **Trade** permission.
2. Set a passphrase. vergex needs it along with the key and secret.
3. Add the server IP to the whitelist.
4. Copy the key, secret and passphrase. The secret is shown only once.

Ranked coin sources use Binance data; for OKX traders, coins that OKX does not list as live USDT swaps are skipped.

## Common errors

| Error | Likely cause |
| :---- | :----------- |
| `Invalid API-key`, `Invalid signature` | Wrong key or secret, or a missing OKX passphrase |
| `IP not whitelisted`, `-2015` | The server IP is not on the key's whitelist |
| `-1021`, `timestamp` errors | The server clock is off: see [troubleshooting.md](troubleshooting.md) |
| `-4061` position side mismatch | Binance account is in One-way Mode |
| `Subaccounts restricted to 5x leverage` | Lower the leverage in the strategy |
| Permission denied | Futures or trade permission is not enabled on the key |
