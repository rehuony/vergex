# vergex web

The vergex frontend: React 18, TypeScript, Vite, Tailwind v4, SWR, lightweight-charts. UI conventions are in the frontend section of [AGENTS.md](../AGENTS.md).

## Commands

The package manager is pnpm, pinned by `packageManager` in `package.json`. Run `corepack enable` once and `pnpm` uses that version.

```bash
pnpm install
pnpm dev      # http://localhost:8000, proxies /api to :3000 (start the backend with make run-backend)
pnpm build    # type-check with tsc, then build to dist/ with Vite
pnpm test     # Vitest unit tests
pnpm lint     # ESLint, including Prettier
pnpm format   # format src/ with Prettier
```

`pnpm-workspace.yaml` holds dependency `overrides` and `allowBuilds`, the dependencies allowed to run install scripts.

## Layout (`src/`)

| Directory | Contents |
| :-------- | :------- |
| `pages/` | Route-level pages, with their own CSS beside them |
| `routes/` | Route table and guards |
| `components/<area>/` | Components by feature; `common/` has page scaffolding such as Page and Dialog, `ui/` has base controls such as Button and AppSelect. A component with its own files gets a directory with an `index.ts` |
| `api/` | HTTP client and API modules |
| `hooks/`, `contexts/` | Shared hooks and React contexts |
| `utils/` | Shared helpers outside components |
| `constants/` | Shared values such as route paths and model providers |
| `i18n/` | UI copy |
| `styles/` | Global styles (`index.css`) and theme variables (`brand.css`) |
| `types/` | Shared types |
| `tests/` | Vitest tests, mirroring the paths above; `setup.ts` runs before them |
