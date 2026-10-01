// Boots Swagger UI against the spec served by this tts-server.
window.addEventListener("load", function () {
  window.ui = SwaggerUIBundle({
    url: "/openapi.yaml",
    dom_id: "#swagger-ui",
    deepLinking: true,
    persistAuthorization: false,
  });
});
