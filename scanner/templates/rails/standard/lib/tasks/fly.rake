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
  task :server => :swapfile do
    sh 'bin/rails server'
  end

  # optional SWAPFILE task:
  #  - adjust fallocate size as needed
  #  - performance critical applications should scale memory to the
  #    point where swap is rarely used.  'fly scale help' for details.
  #  - disable by removing dependency on the :server task, thus:
  #        task :server do
  task :swapfile do
    sh 'fallocate -l 512M /swapfile'
    sh 'chmod 0600 /swapfile'
    sh 'mkswap /swapfile'
    sh 'echo 10 > /proc/sys/vm/swappiness'
    sh 'swapon /swapfile'
  end

  # BUILD step:
  # - Checks that Gemfile.lock matches up with environment.
  # - Displays error messages on how to rectify the environment.
  task :verify do
    lockfile = Bundler::LockfileParser.new(File.read("Gemfile.lock"))
    platform = Gem::Platform.new("x86_64-linux")

    if lockfile.ruby_version.nil?
      fail <<~ERROR
        A Ruby version is not specified in the Gemfile.

        Set the Ruby version in the Gemfile by adding the following to your Gemfile:

        ```
        ruby #{RUBY_VERSION.inspect}
        ```

        Then run `bundle` and deploy again.
      ERROR
    elsif lockfile.ruby_version != RUBY_VERSION
      fail <<~ERROR
        The version of Ruby specified in the Gemfile is #{lockfile.ruby_version.inspect}, which does not match #{RUBY_VERSION.inspect}.

        Set the RUBY_VERSION in the `Dockerfile` file:

        ```
        ARG RUBY_VERSION=#{lockfile.ruby_version}
        ```

        Then deploy again.
      ERROR
    end

    if lockfile.platforms.include? platform
      fail <<~ERROR
        Gemfile.lock does not have the platform #{platform.to_s.inspect}.

        Add the platform by running `bundle lock --add-platform #{platform.to_s}`, then deploy again.
      ERROR
    end
  end
end
