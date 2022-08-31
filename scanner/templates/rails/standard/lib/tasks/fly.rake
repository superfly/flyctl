# commands used to deploy a Rails application
namespace :fly do
  # BUILD step:
  #  - changes to the filesystem made here DO get deployed
  #  - NO access to secrets, volumes, databases
  #  - Failures here prevent deployment
  task :build => 'assets:precompile'

  # RELEASE step:
  #  - changes to the filesystem made here are DISCARDED
  #  - full access to secrets, databases
  #  - failures here prevent deployment
  task :release => 'db:migrate'

  # SERVER step:
  #  - changes to the filesystem made here are deployed
  #  - full access to secrets, databases
  #  - failures here result in VM being stated, shutdown, and rolled back
  #    to last successful deploy (if any).
  task :server do
    sh 'bin/rails server'
  end
end
