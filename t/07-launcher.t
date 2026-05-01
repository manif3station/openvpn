#!/usr/bin/env perl
use strict;
use warnings;

use File::Path qw(make_path);
use File::Spec;
use File::Temp qw(tempdir);
use Test::More;

use lib 'lib';
use OpenVPN::Launcher;

sub fake_launcher {
    my (%args) = @_;
    my $home = tempdir( CLEANUP => 1 );
    my $run  = File::Spec->catdir( $home, '.openvpn-dd' );
    make_path($run);

    my $state = {
        pids     => {},
        next_pid => 3000,
    };

    my $launcher = OpenVPN::Launcher->new(
        home        => $home,
        run_dir     => $run,
        osname      => $args{osname} || 'linux',
        openvpn_bin => $args{openvpn_bin},
        env         => $args{env} || {},
        system      => $args{system} || sub {
            my (@cmd) = @_;
            return $args{system_rc} if defined $args{system_rc};
            my %at;
            for ( my $i = 0; $i <= $#cmd; $i++ ) {
                my $item = $cmd[$i];
                if ( $item eq '--writepid' || $item eq '--log' ) {
                    $at{$item} = $cmd[ $i + 1 ];
                    $i++;
                }
            }
            my $pid = ++$state->{next_pid};
            open my $pfh, '>', $at{'--writepid'} or die $!;
            print {$pfh} "$pid\n";
            close $pfh or die $!;
            open my $lfh, '>', $at{'--log'} or die $!;
            print {$lfh} "Initialization Sequence Completed\n";
            close $lfh or die $!;
            $state->{pids}{$pid} = 1;
            return 0;
        },
        spawner => $args{spawner} || sub {
            my (@cmd) = @_;
            my $pid = ++$state->{next_pid};
            open my $pfh, '>', File::Spec->catfile( $run, 'openvpn.pid' ) or die $!;
            print {$pfh} "$pid\n";
            close $pfh or die $!;
            open my $lfh, '>', File::Spec->catfile( $run, 'openvpn.log' ) or die $!;
            print {$lfh} "Initialization Sequence Completed\n";
            close $lfh or die $!;
            $state->{pids}{$pid} = 1;
            return $pid;
        },
        capture => $args{capture} || sub {
            my ($cmd) = @_;
            if ( $cmd =~ /^tasklist / ) {
                my ($pid) = $cmd =~ /PID eq (\d+)/;
                return exists $state->{pids}{$pid} ? "openvpn.exe $pid\n" : "INFO: No tasks are running\n";
            }
            if ( $cmd =~ /^taskkill / ) {
                my ($pid) = $cmd =~ /PID (\d+)/;
                delete $state->{pids}{$pid};
                return "SUCCESS: Sent termination signal to process with PID $pid.\n";
            }
            return q{};
        },
        killer => sub {
            my ( $signal, $pid ) = @_;
            return exists $state->{pids}{$pid} ? 1 : 0 if defined $signal && ( $signal eq '0' || $signal =~ /^\d+\z/ && $signal == 0 );
            delete $state->{pids}{$pid};
            return 1;
        },
        sleep => sub { return 1 },
    );

    return ( $launcher, $home, $state );
}

{
    my ( $launcher ) = fake_launcher(
        openvpn_bin => undef,
        env         => { OPENVPN_BIN => '/env/openvpn' },
    );
    is( $launcher->openvpn_bin({}), '/env/openvpn', 'launcher prefers process environment override' );
}

{
    my ( $launcher ) = fake_launcher( openvpn_bin => undef );
    is( $launcher->openvpn_bin( { OPENVPN_BIN => '/file/openvpn' } ), '/file/openvpn', 'launcher falls back to env file override' );
    is( $launcher->openvpn_bin({}), 'openvpn', 'launcher falls back to plain openvpn last' );
}

{
    my ( $launcher ) = fake_launcher(
        osname      => 'MSWin32',
        openvpn_bin => undef,
        env         => { ProgramFiles => 'C:/Program Files' },
    );
    is( $launcher->openvpn_bin({}), 'openvpn.exe', 'launcher falls back to openvpn.exe on Windows when no explicit path exists' );
}

{
    my $home = tempdir( CLEANUP => 1 );
    my $launcher = OpenVPN::Launcher->new( home => $home, env => {} );
    is( $launcher->{system}->('true'), 0, 'launcher default system helper returns zero for true' );
    ok( $launcher->{spawner}->('perl', '-e', 'exit 0') > 0, 'launcher default spawner returns a child pid' );
    ok( defined $launcher->{capture}->('printf qx-capture'), 'launcher default capture helper returns command output' );
    $launcher->{sleep}->(0);
    pass('launcher default sleep helper runs without error');
    is( $launcher->{killer}->( 0, $$ ), 1, 'launcher default killer helper can probe the current process' );
    is( $launcher->run_dir, File::Spec->catdir( $home, '.openvpn-dd' ), 'launcher default run_dir falls back under the home directory' );
}

{
    is(
        OpenVPN::Launcher->_spawn_dispatch( 'MSWin32', sub { my ( $mode, @cmd ) = @_; return $mode == 1 ? 6789 : 0 }, 'openvpn.exe' ),
        6789,
        'launcher dispatches Windows spawns through the native callback'
    );
}

{
    ok(
        OpenVPN::Launcher->_spawn_windows( undef, 'perl', '-e', 'exit 0' ) > 0,
        'launcher Windows spawn falls back to the Perl-managed async spawn path when no native callback is supplied'
    );
}

{
    my ( $launcher ) = fake_launcher( with_env => 1 );
    my $pid = $launcher->start(
        env       => {},
        config    => '/tmp/config.ovpn',
        auth_file => '/tmp/auth.txt',
    );
    ok( $pid > 0, 'launcher start returns a pid' );
    ok( $launcher->is_connected, 'launcher reports connected after start' );
}

{
    my ( $launcher ) = fake_launcher( system_rc => 1 );
    my $ok = eval {
        $launcher->start(
            env       => {},
            config    => '/tmp/config.ovpn',
            auth_file => '/tmp/auth.txt',
        );
        1;
    };
    ok( !$ok, 'launcher start rejects nonzero openvpn exit codes' );
    like( $@, qr/openvpn failed with exit code 1/, 'launcher reports openvpn launch failures clearly' );
}

{
    my ( $launcher ) = fake_launcher;
    $launcher->start(
        env       => {},
        config    => '/tmp/config.ovpn',
        auth_file => '/tmp/auth.txt',
    );
    my $stopped = $launcher->stop;
    ok( $stopped > 0, 'launcher stop returns the stopped pid' );
    ok( !$launcher->is_connected, 'launcher stop clears the live process state' );
}

{
    my ( $launcher ) = fake_launcher( osname => 'MSWin32' );
    my $pid = $launcher->start(
        env       => {},
        config    => 'C:/vpn/config.ovpn',
        auth_file => 'C:/vpn/auth.txt',
    );
    ok( $pid > 0, 'launcher start returns a pid on Windows' );
    ok( $launcher->is_connected, 'launcher reports connected after Windows spawn' );
    ok( $launcher->stop > 0, 'launcher stop returns the Windows pid' );
    ok( !$launcher->is_connected, 'launcher stop clears the Windows process state' );
}

{
    my ( $launcher ) = fake_launcher(
        osname  => 'MSWin32',
        spawner => sub { return 4321 },
        capture => sub {
            my ($cmd) = @_;
            return $cmd =~ /^tasklist / ? "openvpn.exe 4321\n" : q{};
        },
    );
    my $pid = $launcher->start(
        env       => {},
        config    => 'C:/vpn/config.ovpn',
        auth_file => 'C:/vpn/auth.txt',
    );
    is( $pid, 4321, 'Windows launcher start preserves the spawned pid when it has to write the pid file itself' );
    ok( -f $launcher->pid_file, 'Windows launcher writes the pid file when the spawner does not' );
}

done_testing;
