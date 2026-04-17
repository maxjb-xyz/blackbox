package handlers

import "net/http"

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
  <script id="api-reference"></script>
  <script>
    var ref = document.getElementById('api-reference');
    ref.dataset.url = '/api/openapi.yaml';
    ref.dataset.configuration = JSON.stringify({
      theme: 'saturn',
      darkMode: true,
      defaultHttpClient: { targetKey: 'shell', clientKey: 'curl' },
      servers: [{ url: window.location.origin, description: 'This server' }],
    });
  </script>
  <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`

// OpenAPISpec serves the raw OpenAPI YAML spec.
func OpenAPISpec(spec []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(spec)
	}
}

// APIDocs serves the interactive Scalar API reference UI.
func APIDocs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(scalarHTML))
	}
}
