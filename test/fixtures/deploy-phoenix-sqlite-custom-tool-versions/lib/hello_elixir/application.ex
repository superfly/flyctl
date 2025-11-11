defmodule HelloElixir.Application do
  # See https://hexdocs.pm/elixir/Application.html
  # for more information on OTP Applications
  @moduledoc false

  use Application

  @impl true
  def start(_type, _args) do
    children = [
      # Start the Ecto repository
      HelloElixir.Repo,
      # Start the Telemetry supervisor
      HelloElixirWeb.Telemetry,
      # Start the PubSub system
      {Phoenix.PubSub, name: HelloElixir.PubSub},
      # Start the Endpoint (http/https)
      HelloElixirWeb.Endpoint
      # Start a worker by calling: HelloElixir.Worker.start_link(arg)
      # {HelloElixir.Worker, arg}
    ]

    # See https://hexdocs.pm/elixir/Supervisor.html
    # for other strategies and supported options
    opts = [strategy: :one_for_one, name: HelloElixir.Supervisor]
    Supervisor.start_link(children, opts)
  end

  # Tell Phoenix to update the endpoint configuration
  # whenever the application is updated.
  @impl true
  def config_change(changed, _new, removed) do
    HelloElixirWeb.Endpoint.config_change(changed, removed)
    :ok
  end
end
