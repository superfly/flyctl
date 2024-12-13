defmodule DeployPhoenixSqlite.Repo do
  use Ecto.Repo,
    otp_app: :deploy_phoenix_sqlite,
    adapter: Ecto.Adapters.SQLite3
end
