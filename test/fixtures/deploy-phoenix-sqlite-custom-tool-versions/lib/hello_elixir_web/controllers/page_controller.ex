defmodule HelloElixirWeb.PageController do
  use HelloElixirWeb, :controller

  def index(conn, _params) do
    render(conn, "index.html")
  end
end
