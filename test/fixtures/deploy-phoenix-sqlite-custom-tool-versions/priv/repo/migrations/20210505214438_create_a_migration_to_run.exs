defmodule HelloElixir.Repo.Migrations.CreateAMigrationToRun do
  use Ecto.Migration

  def change do
    create table(:testing) do
      add :name, :string
    end
  end
end
