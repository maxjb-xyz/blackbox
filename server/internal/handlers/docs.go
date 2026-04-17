package handlers

import "net/http"

// scalarCDN is the pinned Scalar API reference bundle URL.
// Update this and the integrity hash together when upgrading.
const scalarCDN = "https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.25.0/dist/browser/standalone.min.js"

const scalarHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Blackbox API</title>
  <style>
    body { margin: 0; padding: 0; }
  </style>
</head>
<body>
  <script
    id="api-reference"
    data-url="/api/openapi.yaml"
    data-configuration='{"theme":"saturn","darkMode":true,"defaultHttpClient":{"targetKey":"shell","clientKey":"curl"}}'
  ></script>
  <script src="` + scalarCDN + `" crossorigin="anonymous"></script>
</body>
</html>`

// docsCSP is a per-route Content-Security-Policy that extends the global policy
// to allow the Scalar CDN script. The inline script is eliminated by using
// data-configuration attributes instead.
const docsCSP = "default-src 'self'; script-src 'self' https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' https://cdn.jsdelivr.net; connect-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

// OpenAPISpec serves the raw OpenAPI YAML spec.
func OpenAPISpec(spec []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(spec)
	}
}

// APIDocs serves the interactive Scalar API reference UI.
func APIDocs() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Security-Policy", docsCSP)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(scalarHTML))
	}
}
