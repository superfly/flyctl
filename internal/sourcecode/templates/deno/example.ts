import {
  app,
  get,
  post,
  redirect,
  contentType,
} from "https://denopkg.com/syumai/dinatra/mod.ts";

const greeting = "<h1>Hello From Deno on Fly!</h1>";

app(
  get("/", () => greeting),
  get("/:id", ({ params }) => greeting + `</br>and hello to ${params.id}`),
);
