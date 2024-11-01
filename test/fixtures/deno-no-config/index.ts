Deno.serve({ port: 8080, hostname: "0.0.0.0" }, (_req) => {
    return new Response("Hello, World!");
});