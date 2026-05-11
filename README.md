# MeshHub

Desktop asset transcoder scaffold using Vue, Tailwind CSS, Go, and Wails.

## Layout

- `Frotend_Vue/`: Vue frontend source. Vue files should stay in this folder.
- `style/`: Tailwind and global CSS.
- `backend_go/`: Go and Wails application code.
- `server/`: server-side model conversion and processing code.
- `export_app/`: packaged application exports.

## Commands

```bash
pnpm install
pnpm frontend:dev
pnpm frontend:build
pnpm server:dev
pnpm wails:dev
pnpm wails:build
```

`pnpm wails:build` builds the Vue frontend, runs the Wails build from `backend_go`, and copies build output into `export_app`.
