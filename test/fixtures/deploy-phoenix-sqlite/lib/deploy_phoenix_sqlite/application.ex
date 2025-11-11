defmodule DeployPhoenixSqlite.Application do
  # See https://hexdocs.pm/elixir/Application.html
  # for more information on OTP Applications
  @moduledoc false

  use Application

  @impl true
  def start(_type, _args) do
    children = [
      DeployPhoenixSqliteWeb.Telemetry,
      DeployPhoenixSqlite.Repo,
      {Ecto.Migrator,
        repos: Application.fetch_env!(:deploy_phoenix_sqlite, :ecto_repos),
        skip: skip_migrations?()},
      {DNSCluster, query: Application.get_env(:deploy_phoenix_sqlite, :dns_cluster_query) || :ignore},
      {Phoenix.PubSub, name: DeployPhoenixSqlite.PubSub},
      # Start the Finch HTTP client for sending emails
      {Finch, name: DeployPhoenixSqlite.Finch},
      # Start a worker by calling: DeployPhoenixSqlite.Worker.start_link(arg)
      # {DeployPhoenixSqlite.Worker, arg},
      # Start to serve requests, typically the last entry
      DeployPhoenixSqliteWeb.Endpoint
    ]

    # See https://hexdocs.pm/elixir/Supervisor.html
    # for other strategies and supported options
    opts = [strategy: :one_for_one, name: DeployPhoenixSqlite.Supervisor]
    Supervisor.start_link(children, opts)
  end

  # Tell Phoenix to update the endpoint configuration
  # whenever the application is updated.
  @impl true
  def config_change(changed, _new, removed) do
    DeployPhoenixSqliteWeb.Endpoint.config_change(changed, removed)
    :ok
  end

  defp skip_migrations?() do
    # By default, sqlite migrations are run when using a release
    System.get_env("RELEASE_NAME") != nil
  end
end
