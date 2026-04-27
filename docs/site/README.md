# Blackbox Docs

This directory contains the standalone Docusaurus site for Blackbox documentation.

## Local development

Recommended Node.js version: `22.16.0`

Install dependencies:

```bash
npm install
```

Start the local dev server:

```bash
npm start
```

Create a production build:

```bash
npm run build
```

Serve the built site locally:

```bash
npm run serve
```

## Cloudflare Pages

Deploy this site to Cloudflare Pages with these settings:

- Custom domain: `docs.blackboxd.dev`
- Root directory: `docs/site`
- Build command: `npm run build`
- Build output directory: `build`

Cloudflare Pages installs dependencies before the build step runs, so the build command does not need to include `npm ci`.

This scaffold includes a `.node-version` file pinned to Node `22.16.0`, which matches Cloudflare Pages' supported versioning approach and avoids the known Docusaurus build issues seen on local Node `25.x`.
