package OpenVPN::Manager;

use strict;
use warnings;

use File::Basename qw(dirname);
use File::Path qw(make_path);
use File::Spec;
use JSON::PP qw(decode_json encode_json);
use OpenVPN::TOTP;

sub new {
    my ( $class, %args ) = @_;
    my $self = bless {
        home          => $args{home} || $ENV{HOME},
        env           => $args{env} || \%ENV,
        system        => $args{system} || sub { return system(@_) >> 8 },
        sleep         => $args{sleep} || sub { sleep $_[0] },
        now           => $args{now} || sub { return time },
        stdin_fh      => $args{stdin_fh} || \*STDIN,
        stdout_fh     => $args{stdout_fh} || \*STDOUT,
        stderr_fh     => $args{stderr_fh} || \*STDERR,
        interactive   => exists $args{interactive} ? $args{interactive} : -t STDIN,
        skill_root    => $args{skill_root},
        run_dir       => $args{run_dir},
        config_path   => $args{config_path},
        openvpn_bin   => $args{openvpn_bin},
        killer        => $args{killer} || sub { my ( $signal, $pid ) = @_; return kill $signal, $pid },
    }, $class;
    return $self;
}

sub main_setup {
    my ( $class, @argv ) = @_;
    my $self = ref($class) ? $class : $class->new;
    my $result = eval { $self->execute_setup(@argv) };
    if ( my $error = $@ ) {
        chomp $error;
        print { $self->{stderr_fh} } "$error\n";
        return 2;
    }
    print { $self->{stdout_fh} } encode_json($result) . "\n";
    return 0;
}

sub main_connect {
    my ( $class, @argv ) = @_;
    my $self = ref($class) ? $class : $class->new;
    my $result = eval { $self->execute_connect(@argv) };
    if ( my $error = $@ ) {
        chomp $error;
        print { $self->{stderr_fh} } "$error\n";
        return 2;
    }
    print { $self->{stdout_fh} } encode_json($result) . "\n";
    return $self->result_exit_code($result);
}

sub main_disconnect {
    my ( $class, @argv ) = @_;
    my $self = ref($class) ? $class : $class->new;
    my $result = eval { $self->execute_disconnect(@argv) };
    if ( my $error = $@ ) {
        chomp $error;
        print { $self->{stderr_fh} } "$error\n";
        return 2;
    }
    print { $self->{stdout_fh} } encode_json($result) . "\n";
    return 0;
}

sub main_noreconnect {
    my ( $class, @argv ) = @_;
    my $self = ref($class) ? $class : $class->new;
    my $result = eval { $self->execute_noreconnect(@argv) };
    if ( my $error = $@ ) {
        chomp $error;
        print { $self->{stderr_fh} } "$error\n";
        return 2;
    }
    print { $self->{stdout_fh} } encode_json($result) . "\n";
    return 0;
}

sub execute_setup {
    my ( $self, @argv ) = @_;
    my $opt = $self->parse_setup_args(@argv);
    $self->ensure_runtime_dir;

    my $existing = $self->read_env_file;
    my $username = defined $opt->{username} ? $opt->{username} : $existing->{OPENVPN_USERNAME};
    my $password = defined $opt->{password} ? $opt->{password} : $existing->{OPENVPN_PASSWORD};
    my $twofa    = defined $opt->{twofa}    ? $opt->{twofa}    : $existing->{OPENVPN_2FA};

    $username = $self->prompt_visible('OpenVPN username: ') if !defined $username || $username eq q{};
    $password = $self->prompt_hidden('OpenVPN password: ')  if !defined $password || $password eq q{};
    $twofa    = $self->prompt_hidden('OpenVPN 2FA token or secret (optional): ') if !defined $twofa;
    $twofa = q{} if !defined $twofa;

    die "OpenVPN username is required\n" if !defined $username || $username eq q{};
    die "OpenVPN password is required\n" if !defined $password || $password eq q{};

    my %merged = (
        %{$existing},
        OPENVPN_USERNAME => $username,
        OPENVPN_PASSWORD => $password,
    );
    if ( $twofa ne q{} ) {
        $merged{OPENVPN_2FA} = $twofa;
    }
    else {
        delete $merged{OPENVPN_2FA};
    }

    $self->write_env_file( \%merged );
    $self->write_state(
        {
            auto_reconnect => JSON::PP::true,
            retry_count    => 0,
            status         => 'setup',
        }
    );

    return {
        mode             => 'setup',
        env_file         => $self->env_file,
        username         => $username,
        two_factor       => $twofa eq q{} ? 'disabled' : $self->twofa_mode($twofa),
        auto_reconnect   => JSON::PP::true,
        config_candidate => $self->find_openvpn_config || q{},
    };
}

sub execute_connect {
    my ( $self, @argv ) = @_;
    my $opt = $self->parse_connect_args(@argv);
    $self->ensure_runtime_dir;

    my $env = $self->read_env_file;
    if ( !$self->is_setup_complete($env) ) {
        return $self->status_result(
            mode           => $opt->{collector} ? 'collector' : 'connect',
            status         => 'not_setup',
            status_icon    => '?',
            connected      => JSON::PP::false,
            auto_reconnect => JSON::PP::false,
            retry_count    => 0,
            message        => 'Run dashboard openvpn.setup first',
        );
    }

    my $state = $self->read_state;
    if ( $opt->{auto} ) {
        $state->{auto_reconnect} = JSON::PP::true;
        $state->{retry_count}    = 0;
        $self->write_state($state);
    }
    elsif ( !$opt->{collector} ) {
        $state->{auto_reconnect} = JSON::PP::false;
        $state->{retry_count}    = 0;
        $self->write_state($state);
    }

    if ( $self->is_connected ) {
        return $self->status_result(
            mode           => $opt->{collector} ? 'collector' : 'connect',
            status         => 'connected',
            status_icon    => '+',
            connected      => JSON::PP::true,
            auto_reconnect => $self->read_state->{auto_reconnect} ? JSON::PP::true : JSON::PP::false,
            retry_count    => $self->read_state->{retry_count} || 0,
            config         => $self->resolved_openvpn_config($env),
        );
    }

    if ( $opt->{collector} && !$state->{auto_reconnect} ) {
        return $self->status_result(
            mode           => 'collector',
            status         => 'reconnect_disabled',
            status_icon    => '-',
            connected      => JSON::PP::false,
            auto_reconnect => JSON::PP::false,
            retry_count    => $state->{retry_count} || 0,
            config         => $self->resolved_openvpn_config($env),
        );
    }

    my $attempt = eval { $self->start_connection($env) };
    if ($@) {
        my $error = $@;
        chomp $error;
        return $self->handle_connect_failure(
            mode      => $opt->{collector} ? 'collector' : 'connect',
            auto      => $opt->{collector} || $opt->{auto},
            env       => $env,
            state     => $state,
            error_msg => $error,
        );
    }

    $self->write_state(
        {
            auto_reconnect => $opt->{collector}
              ? ( $state->{auto_reconnect} ? JSON::PP::true : JSON::PP::false )
              : ( $opt->{auto} ? JSON::PP::true : JSON::PP::false ),
            retry_count => 0,
            status      => 'connected',
        }
    );

    return $self->status_result(
        mode           => $opt->{collector} ? 'collector' : 'connect',
        status         => $opt->{collector} ? 'reconnected' : 'connected',
        status_icon    => '+',
        connected      => JSON::PP::true,
        auto_reconnect => $opt->{collector}
          ? ( $state->{auto_reconnect} ? JSON::PP::true : JSON::PP::false )
          : ( $opt->{auto} ? JSON::PP::true : JSON::PP::false ),
        retry_count => 0,
        config      => $self->resolved_openvpn_config($env),
        pid         => $self->current_pid || 0,
        auth_file   => $self->auth_file,
    );
}

sub execute_disconnect {
    my ( $self, @argv ) = @_;
    die "Unsupported option: $argv[0]\n" if @argv;
    $self->ensure_runtime_dir;

    my $stopped = $self->stop_connection;
    $self->write_state(
        {
            auto_reconnect => JSON::PP::false,
            retry_count    => 0,
            status         => 'disconnected',
        }
    );

    return $self->status_result(
        mode           => 'disconnect',
        status         => 'disconnected',
        status_icon    => 'x',
        connected      => JSON::PP::false,
        auto_reconnect => JSON::PP::false,
        retry_count    => 0,
        stopped_pid    => $stopped || 0,
    );
}

sub execute_noreconnect {
    my ( $self, @argv ) = @_;
    die "Unsupported option: $argv[0]\n" if @argv;
    $self->ensure_runtime_dir;
    my $state = $self->read_state;
    $state->{auto_reconnect} = JSON::PP::false;
    $state->{status} = 'reconnect_disabled';
    $self->write_state($state);

    return $self->status_result(
        mode           => 'noreconnect',
        status         => 'reconnect_disabled',
        status_icon    => '-',
        connected      => $self->is_connected ? JSON::PP::true : JSON::PP::false,
        auto_reconnect => JSON::PP::false,
        retry_count    => $state->{retry_count} || 0,
    );
}

sub parse_setup_args {
    my ( $self, @argv ) = @_;
    my %opt;
    while (@argv) {
        my $arg = shift @argv;
        if ( $arg eq '-u' || $arg eq '--username' ) {
            die "Missing value after $arg\n" if !@argv;
            $opt{username} = shift @argv;
            next;
        }
        if ( $arg eq '-p' || $arg eq '--password' ) {
            die "Missing value after $arg\n" if !@argv;
            $opt{password} = shift @argv;
            next;
        }
        if ( $arg eq '-2fa' || $arg eq '--2fa' || $arg eq '--token' ) {
            die "Missing value after $arg\n" if !@argv;
            $opt{twofa} = shift @argv;
            next;
        }
        die "Unsupported option: $arg\n";
    }
    return \%opt;
}

sub parse_connect_args {
    my ( $self, @argv ) = @_;
    my %opt = (
        auto      => 0,
        collector => 0,
    );
    while (@argv) {
        my $arg = shift @argv;
        if ( $arg eq '--auto' ) {
            $opt{auto} = 1;
            next;
        }
        if ( $arg eq '--collector' ) {
            $opt{collector} = 1;
            next;
        }
        die "Unsupported option: $arg\n";
    }
    return \%opt;
}

sub prompt_visible {
    my ( $self, $message ) = @_;
    print { $self->{stdout_fh} } $message;
    my $line = readline( $self->{stdin_fh} );
    $line = q{} if !defined $line;
    chomp $line;
    return $line;
}

sub prompt_hidden {
    my ( $self, $message ) = @_;
    return $self->prompt_visible($message) if !$self->{interactive};
    print { $self->{stdout_fh} } $message;
    my $rc = $self->{system}->( 'stty', '-echo' );
    my $line = readline( $self->{stdin_fh} );
    $self->{system}->( 'stty', 'echo' );
    print { $self->{stdout_fh} } "\n";
    $line = q{} if !defined $line;
    chomp $line;
    return $line;
}

sub ensure_runtime_dir {
    my ($self) = @_;
    make_path( $self->run_dir ) if !-d $self->run_dir;
    return $self->run_dir;
}

sub run_dir {
    my ($self) = @_;
    return $self->{run_dir} if defined $self->{run_dir};
    return File::Spec->catdir( $self->{home}, '.openvpn-dd' );
}

sub env_file {
    my ($self) = @_;
    return File::Spec->catfile( $self->{home}, '.openvpn.env' );
}

sub state_file {
    my ($self) = @_;
    return File::Spec->catfile( $self->run_dir, 'state.json' );
}

sub auth_file {
    my ($self) = @_;
    return File::Spec->catfile( $self->run_dir, 'auth.txt' );
}

sub pid_file {
    my ($self) = @_;
    return File::Spec->catfile( $self->run_dir, 'openvpn.pid' );
}

sub log_file {
    my ($self) = @_;
    return File::Spec->catfile( $self->run_dir, 'openvpn.log' );
}

sub read_env_file {
    my ($self) = @_;
    my %env;
    my $path = $self->env_file;
    return \%env if !-f $path;
    open my $fh, '<', $path or die "Unable to read $path: $!";
    while ( my $line = <$fh> ) {
        chomp $line;
        next if $line =~ /^\s*#/;
        next if $line !~ /\A([A-Z0-9_]+)=(.*)\z/;
        my ( $key, $value ) = ( $1, $2 );
        $value =~ s/\A['"]//;
        $value =~ s/['"]\z//;
        $env{$key} = $value;
    }
    close $fh or die "Unable to close $path: $!";
    return \%env;
}

sub write_env_file {
    my ( $self, $env ) = @_;
    my $path = $self->env_file;
    open my $fh, '>', $path or die "Unable to write $path: $!";
    for my $key ( sort keys %{$env} ) {
        next if !defined $env->{$key} || $env->{$key} eq q{};
        print {$fh} "$key=$env->{$key}\n";
    }
    close $fh or die "Unable to close $path: $!";
    chmod 0600, $path;
    return $path;
}

sub read_state {
    my ($self) = @_;
    my $path = $self->state_file;
    return {
        auto_reconnect => JSON::PP::true,
        retry_count    => 0,
        status         => 'new',
    } if !-f $path;
    open my $fh, '<', $path or die "Unable to read $path: $!";
    local $/;
    my $json = <$fh>;
    close $fh or die "Unable to close $path: $!";
    return decode_json( $json || '{}' );
}

sub write_state {
    my ( $self, $state ) = @_;
    my $path = $self->state_file;
    $self->ensure_runtime_dir;
    open my $fh, '>', $path or die "Unable to write $path: $!";
    print {$fh} encode_json($state);
    close $fh or die "Unable to close $path: $!";
    chmod 0600, $path;
    return $path;
}

sub is_setup_complete {
    my ( $self, $env ) = @_;
    return 0 if !defined $env->{OPENVPN_USERNAME} || $env->{OPENVPN_USERNAME} eq q{};
    return 0 if !defined $env->{OPENVPN_PASSWORD} || $env->{OPENVPN_PASSWORD} eq q{};
    return 1;
}

sub resolved_openvpn_config {
    my ( $self, $env ) = @_;
    $env ||= $self->read_env_file;
    my $from_env = $self->{env}{OPENVPN_CONFIG} || $env->{OPENVPN_CONFIG} || q{};
    return $from_env if $from_env ne q{} && -f $self->expand_tilde($from_env);
    my $candidate = $self->find_openvpn_config;
    die "OpenVPN config file not found. Set OPENVPN_CONFIG in ~/.openvpn.env or place one .ovpn file under ~/.openvpn or ~/.config/openvpn\n"
      if !defined $candidate || $candidate eq q{};
    return $candidate;
}

sub find_openvpn_config {
    my ($self) = @_;
    my @candidates = (
        '~/.openvpn/config.ovpn',
        '~/.openvpn/client.ovpn',
        '~/.config/openvpn/client.ovpn',
        '~/.config/openvpn/config.ovpn',
    );
    for my $pattern (@candidates) {
        my $path = $self->expand_tilde($pattern);
        return $path if -f $path;
    }
    for my $dir ( map { $self->expand_tilde($_) } qw(~/.openvpn ~/.config/openvpn) ) {
        next if !-d $dir;
        opendir my $dh, $dir or next;
        my @files = sort grep { /\.ovpn\z/ && -f File::Spec->catfile( $dir, $_ ) } readdir $dh;
        closedir $dh;
        return File::Spec->catfile( $dir, $files[0] ) if @files;
    }
    return q{};
}

sub start_connection {
    my ( $self, $env ) = @_;
    my $config = $self->resolved_openvpn_config($env);
    my $auth_path = $self->write_auth_file($env);
    unlink $self->pid_file if -f $self->pid_file && !$self->pid_alive( $self->current_pid );
    my @cmd = (
        $self->openvpn_bin($env),
        '--daemon',
        '--writepid', $self->pid_file,
        '--log',      $self->log_file,
        '--auth-user-pass', $auth_path,
        '--config',   $config,
        '--auth-nocache',
    );
    my $rc = $self->{system}->(@cmd);
    die "openvpn failed with exit code $rc\n" if $rc != 0;
    $self->{sleep}->(1);
    my $pid = $self->current_pid;
    die "OpenVPN did not create a pid file\n" if !$pid;
    die "OpenVPN process is not running after connect attempt\n" if !$self->pid_alive($pid);
    return $pid;
}

sub write_auth_file {
    my ( $self, $env ) = @_;
    my $path = $self->auth_file;
    my $password = $env->{OPENVPN_PASSWORD};
    my $token = $env->{OPENVPN_2FA} || q{};
    $password .= $self->current_twofa_code($token) if $token ne q{};
    open my $fh, '>', $path or die "Unable to write $path: $!";
    print {$fh} "$env->{OPENVPN_USERNAME}\n$password\n";
    close $fh or die "Unable to close $path: $!";
    chmod 0600, $path;
    return $path;
}

sub current_twofa_code {
    my ( $self, $token ) = @_;
    my $totp = OpenVPN::TOTP->new( now => $self->{now} );
    return $totp->code_for($token);
}

sub openvpn_bin {
    my ( $self, $env ) = @_;
    return $self->{openvpn_bin} if defined $self->{openvpn_bin} && $self->{openvpn_bin} ne q{};
    return $self->{env}{OPENVPN_BIN} if defined $self->{env}{OPENVPN_BIN} && $self->{env}{OPENVPN_BIN} ne q{};
    return $env->{OPENVPN_BIN} if defined $env->{OPENVPN_BIN} && $env->{OPENVPN_BIN} ne q{};
    return 'openvpn';
}

sub current_pid {
    my ($self) = @_;
    my $path = $self->pid_file;
    return 0 if !-f $path;
    open my $fh, '<', $path or return 0;
    my $pid = <$fh>;
    close $fh;
    chomp $pid if defined $pid;
    return $pid && $pid =~ /^\d+\z/ ? $pid : 0;
}

sub pid_alive {
    my ( $self, $pid ) = @_;
    return 0 if !$pid;
    return $self->{killer}->( 0, $pid ) ? 1 : 0;
}

sub is_connected {
    my ($self) = @_;
    my $pid = $self->current_pid;
    return $self->pid_alive($pid);
}

sub stop_connection {
    my ($self) = @_;
    my $pid = $self->current_pid;
    if ( $pid && $self->pid_alive($pid) ) {
        $self->{killer}->( 'TERM', $pid );
        for ( 1 .. 5 ) {
            last if !$self->pid_alive($pid);
            $self->{sleep}->(1);
        }
    }
    unlink $self->pid_file if -f $self->pid_file;
    return $pid;
}

sub handle_connect_failure {
    my ( $self, %args ) = @_;
    my $state = $args{state} || $self->read_state;
    my $auto  = $args{auto} ? 1 : 0;
    my $retry = ( $state->{retry_count} || 0 ) + 1;
    my $disabled = 0;
    if ($auto) {
        if ( $retry >= 5 ) {
            $state->{auto_reconnect} = JSON::PP::false;
            $state->{retry_count} = $retry;
            $state->{status} = 'reconnect_disabled';
            $disabled = 1;
        }
        else {
            $state->{auto_reconnect} = JSON::PP::true;
            $state->{retry_count} = $retry;
            $state->{status} = 'retrying';
        }
    }
    else {
        $state->{auto_reconnect} = JSON::PP::false;
        $state->{retry_count} = 0;
        $state->{status} = 'failed';
    }
    $self->write_state($state);

    return $self->status_result(
        mode           => $args{mode},
        status         => $disabled ? 'reconnect_disabled' : $auto ? 'retrying' : 'failed',
        status_icon    => $disabled ? '-' : '!',
        connected      => JSON::PP::false,
        auto_reconnect => $state->{auto_reconnect} ? JSON::PP::true : JSON::PP::false,
        retry_count    => $state->{retry_count},
        config         => eval { $self->resolved_openvpn_config( $args{env} ) } || q{},
        message        => $args{error_msg},
    );
}

sub status_result {
    my ( $self, %args ) = @_;
    return {
        mode           => $args{mode},
        status         => $args{status},
        status_icon    => $args{status_icon},
        connected      => $args{connected},
        auto_reconnect => $args{auto_reconnect},
        retry_count    => $args{retry_count},
        env_file       => $self->env_file,
        state_file     => $self->state_file,
        pid_file       => $self->pid_file,
        config         => $args{config} || q{},
        message        => $args{message} || q{},
        pid            => $args{pid} || $self->current_pid || 0,
        ( defined $args{auth_file} ? ( auth_file => $args{auth_file} ) : () ),
        ( defined $args{stopped_pid} ? ( stopped_pid => $args{stopped_pid} ) : () ),
    };
}

sub result_exit_code {
    my ( $self, $result ) = @_;
    return 1 if !$result->{connected};
    return 0;
}

sub twofa_mode {
    my ( $self, $token ) = @_;
    return 'static' if $token =~ /^\d{6}\z/;
    return 'totp';
}

sub expand_tilde {
    my ( $self, $path ) = @_;
    return $self->{home} . substr( $path, 1 ) if defined $path && $path =~ /^~\//;
    return $path;
}

1;
