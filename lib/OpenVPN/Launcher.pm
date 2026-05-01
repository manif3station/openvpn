package OpenVPN::Launcher;

use strict;
use warnings;

use File::Spec;
use POSIX ();

sub new {
    my ( $class, %args ) = @_;
    my $osname = $args{osname} || $^O;
    return bless {
        home        => $args{home} || $ENV{HOME},
        env         => $args{env} || \%ENV,
        run_dir     => $args{run_dir},
        osname      => $osname,
        openvpn_bin => $args{openvpn_bin},
        system      => $args{system} || sub { return system(@_) >> 8 },
        spawner     => $args{spawner} || sub { return $class->_spawn_dispatch( $osname, $args{win32_spawn_native}, @_ ) },
        capture     => $args{capture} || sub { return qx(@_) },
        sleep       => $args{sleep} || sub { sleep $_[0] },
        killer      => $args{killer} || sub { my ( $signal, $pid ) = @_; return kill $signal, $pid },
    }, $class;
}

sub run_dir {
    my ($self) = @_;
    return $self->{run_dir} if defined $self->{run_dir};
    return File::Spec->catdir( $self->{home}, '.openvpn-dd' );
}

sub pid_file {
    my ($self) = @_;
    return File::Spec->catfile( $self->run_dir, 'openvpn.pid' );
}

sub log_file {
    my ($self) = @_;
    return File::Spec->catfile( $self->run_dir, 'openvpn.log' );
}

sub openvpn_bin {
    my ( $self, $env ) = @_;
    return $self->{openvpn_bin} if defined $self->{openvpn_bin} && $self->{openvpn_bin} ne q{};
    return $self->{env}{OPENVPN_BIN} if defined $self->{env}{OPENVPN_BIN} && $self->{env}{OPENVPN_BIN} ne q{};
    return $env->{OPENVPN_BIN} if defined $env->{OPENVPN_BIN} && $env->{OPENVPN_BIN} ne q{};

    if ( $self->is_windows ) {
        for my $path (
            'openvpn.exe',
            $self->windows_candidate( 'ProgramFiles',      'OpenVPN', 'bin', 'openvpn.exe' ),
            $self->windows_candidate( 'ProgramFiles(x86)', 'OpenVPN', 'bin', 'openvpn.exe' ),
        ) {
            next if !defined $path || $path eq q{};
            return $path if $path eq 'openvpn.exe' || -f $path;
        }
    }

    return 'openvpn';
}

sub start {
    my ( $self, %args ) = @_;
    my $env       = $args{env} || {};
    my $config    = $args{config} or die "Missing OpenVPN config path\n";
    my $auth_file = $args{auth_file} or die "Missing OpenVPN auth file path\n";

    unlink $self->pid_file if -f $self->pid_file && !$self->pid_alive( $self->current_pid );

    my @cmd = (
        $self->openvpn_bin($env),
        '--writepid', $self->pid_file,
        '--log',      $self->log_file,
        '--auth-user-pass', $auth_file,
        '--config',   $config,
        '--auth-nocache',
    );

    my $spawned_pid = 0;
    if ( $self->is_windows ) {
        $spawned_pid = $self->{spawner}->(@cmd);
        die "openvpn failed to spawn on Windows\n" if !$spawned_pid;
        $self->write_pid_file($spawned_pid) if !-f $self->pid_file;
    }
    else {
        my $rc = $self->{system}->( $cmd[0], '--daemon', @cmd[ 1 .. $#cmd ] );
        die "openvpn failed with exit code $rc\n" if $rc != 0;
    }

    $self->{sleep}->(1);
    my $pid = $self->current_pid;
    die "OpenVPN did not create a pid file\n" if !$pid;
    die "OpenVPN process is not running after connect attempt\n" if !$self->pid_alive($pid);

    return $pid;
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
    if ( $self->is_windows ) {
        my $output = $self->{capture}->(qq{tasklist //FI "PID eq $pid"});
        return $output =~ /\b\Q$pid\E\b/ ? 1 : 0;
    }
    return $self->{killer}->( 0, $pid ) ? 1 : 0;
}

sub is_connected {
    my ($self) = @_;
    my $pid = $self->current_pid;
    return $self->pid_alive($pid);
}

sub stop {
    my ($self) = @_;
    my $pid = $self->current_pid;
    if ( $pid && $self->pid_alive($pid) ) {
        if ( $self->is_windows ) {
            $self->{capture}->("taskkill /PID $pid /T /F");
        }
        else {
            $self->{killer}->( 'TERM', $pid );
        }
        for ( 1 .. 5 ) {
            last if !$self->pid_alive($pid);
            $self->{sleep}->(1);
        }
    }
    unlink $self->pid_file if -f $self->pid_file;
    return $pid;
}

sub is_windows {
    my ($self) = @_;
    return $self->{osname} eq 'MSWin32' ? 1 : 0;
}

sub windows_candidate {
    my ( $self, $base, @parts ) = @_;
    my $root = $self->{env}{$base};
    return q{} if !defined $root || $root eq q{};
    return File::Spec->catfile( $root, @parts );
}

sub write_pid_file {
    my ( $self, $pid ) = @_;
    open my $fh, '>', $self->pid_file or die "Unable to write " . $self->pid_file . ": $!";
    print {$fh} "$pid\n";
    close $fh or die "Unable to close " . $self->pid_file . ": $!";
    return $self->pid_file;
}

sub _spawn_dispatch {
    my ( $class, $osname, $native, @cmd ) = @_;
    return $class->_spawn_windows( $native, @cmd ) if $osname eq 'MSWin32';
    return $class->_spawn_unix(@cmd);
}

sub _spawn_windows {
    my ( $class, $native, @cmd ) = @_;
    return $native->( 1, @cmd ) if $native;
    return $class->_spawn_unix(@cmd);
}

sub _spawn_unix {
    my ( $class, @cmd ) = @_;
    my $pid = fork();
    die "Unable to fork OpenVPN process: $!" if !defined $pid;
    if ( $pid == 0 ) {
        exec @cmd or POSIX::_exit(127);
    }
    return $pid;
}

1;
