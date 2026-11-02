# Accounts

An account is a set of API keys on a venue. vergex trades **Binance USDT-M futures** and **OKX USDT perpetual swaps**. Add accounts in **Settings → Trading accounts → Add account**. The **Verify** button on an account checks the key against the exchange, and the list shows what the key may do.

Rules for both venues:

- Enable reading and futures trading. **Never enable withdrawals.**
- Restrict the key to your server's IP. Settings → Trading accounts shows it, and `GET /api/system/ip` returns it.
- Prefer a dedicated sub-account to isolate risk. Some sub-accounts have lower leverage limits.
- Try new setups on a [testnet](#testnet) account first, then with a small balance.
- One API key can be connected once. Two accounts on one key would trade the same money without knowing of each other.
- An account with trading history or an unfinished [Futures](futures.md) trade cannot be deleted. Changing its key does not release a reservation.

**API permissions** are what the venue reports for the key: reading, futures trading, withdrawals and whether it is restricted to certain IPs. Binance answers from `GET /sapi/v1/account/apiRestrictions`, except on its test environment, which has no such endpoint. OKX answers from `GET /api/v5/account/config`. The list names what is on, and marks a key that can withdraw in red. A failed read only leaves them out.

Accounts are listed by venue, real before testnet, then by name. An account's name can be changed in its form at any time without disturbing anything using it; its venue and testnet setting can't. Deleting an account also deletes its [Futures](futures.md) conversations and drafts.

An account's form shows its state beside its name, and **Ledger** at the top opens its ledger: the orders and fills recorded on it, whoever placed them, including manual orders and ones placed on the exchange by hand, with a sum by source and CSV export of what is shown. It lists the newest 500 of each, and says so when there are more.

## Binance

1. Go to **Account → API Management → Create API** and choose a system-generated key.
2. Enable **Reading** and **Futures**.
3. Under **Edit restrictions**, select **Restrict access to trusted IPs only** and add the server IP.
4. Copy the API key and secret. The secret is shown only once.

Traders need **Hedge Mode**. vergex sets it when it connects, but Binance refuses the change while you have open positions or orders. If you see `code=-4061`, close them and switch manually: Futures → Preferences → Position Mode → Hedge Mode.

## OKX

1. Go to **Profile → API → Create API key** with the **Trade** permission.
2. Set a passphrase. vergex needs it along with the key and secret.
3. Add the server IP to the whitelist.
4. Copy the key, secret and passphrase. The secret is shown only once.

Ranked coin sources use Binance data; for OKX traders, coins that OKX does not list as live USDT swaps are skipped.

## Testnet

An account can be on a venue's test environment, with test money. Press **Testnet** beside the venue when adding it. It is set then and cannot change, so one account's history never mixes the two.

| Venue | Where the key comes from |
| :---- | :----------------------- |
| Binance | Futures demo trading at [demo.binance.com](https://demo.binance.com/en/futures), under API management. vergex reaches it at `demo-fapi.binance.com` |
| OKX | Trade → Demo Trading → Personal Center → Demo Trading API. A real key on demo trading, or a demo key on a real account, is refused with error `50101` |

The account's balance, positions, orders and fills are all on the test environment, and a [Futures](futures.md) plan on it reads its book there. Everything else stays on the real market: charts, candles, rankings and what a trader's model reads. The two sit close together but not on the same price, so a stop very near the price can be refused.

Wherever an order, a position or a trader on such an account is in question, it says so. The account switcher on the Futures page lists names as they are. Its money does not count towards the Dashboard's total equity and realized today, nor the equity on the Auto trading overview. Its events and positions still show.

A testnet account proves the flow, not a real key's permissions or IP allowlist. Use a small account before real money.

## Common errors

| Error | Likely cause |
| :---- | :----------- |
| `Invalid API-key`, `Invalid signature` | Wrong key or secret, or a missing OKX passphrase |
| `IP not whitelisted`, `-2015` | The server IP is not on the key's whitelist |
| `-1021`, `timestamp` errors | The server clock is off: see [troubleshooting.md](troubleshooting.md) |
| `-4061` position side mismatch | Binance account is in One-way Mode |
| `Subaccounts restricted to 5x leverage` | Lower the leverage in the trader's settings |
| Permission denied | Futures or trade permission is not enabled on the key |
