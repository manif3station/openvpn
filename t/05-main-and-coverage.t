#!/usr/bin/env perl
use strict;
use warnings;

use File::Path qw(make_path);
use File::Spec;
use File::Temp qw(tempdir);
use JSON::PP ();
use Test::More;

use lib 'lib';
use OpenVPN::Manager;

sub fake_manager {
    my (%args) = @_;
    my $home = tempdir( CLEANUP => 1 );
    make_path( File::Spec->catdir( $home, '.openvpn' ) );
    my $profile_name = $args{profile_name} || 'client.ovpn';
    my $ovpn = File::Spec->catfile( $home, '.openvpn', $profile_name );
    open my $cfh, '>', $ovpn or die $!;
    print {$cfh} "client\n";
    close $cfh or die $!;

    my $state = {
        pids => {},
        next_pid => 2000,
        linger_after_term => $args{linger_after_term} || 0,
    };

    my $manager = OpenVPN::Manager->new(
        home        => $home,
        interactive => exists $args{interactive} ? $args{interactive} : 0,
        openvpn_bin => $args{openvpn_bin},
        env         => $args{env} || {},
        stdin_fh    => $args{stdin_fh} || \*STDIN,
        stdout_fh   => $args{stdout_fh} || \*STDOUT,
        stderr_fh   => $args{stderr_fh} || \*STDERR,
        system      => $args{system} || sub {
            my (@cmd) = @_;
            if ( grep { $_ eq '--writepid' } @cmd ) {
                my %at;
                for ( my $i = 0; $i <= $#cmd; $i++ ) {
                    my $item = $cmd[$i];
                    if ( $item eq '--writepid' || $item eq '--log' || $item eq '--auth-user-pass' || $item eq '--config' ) {
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
            }
            return 0;
        },
        killer => sub {
            my ( $signal, $pid ) = @_;
            return exists $state->{pids}{$pid} ? 1 : 0 if defined $signal && ( $signal eq '0' || $signal =~ /^\d+\z/ && $signal == 0 );
            if ( defined $signal && $signal eq 'TERM' ) {
                $state->{term_seen}{$pid} = 1;
                return 1;
            }
            delete $state->{pids}{$pid};
            return 1;
        },
        sleep => sub {
            for my $pid ( keys %{ $state->{term_seen} || {} } ) {
                if ( $state->{linger_after_term} ) {
                    $state->{linger_after_term} = 0;
                    next;
                }
                delete $state->{pids}{$pid};
            }
            return 1;
        },
        now => sub { return 1_700_000_000 },
    );

    $manager->write_env_file(
        {
            OPENVPN_USERNAME => 'alice',
            OPENVPN_PASSWORD => 'secret',
            OPENVPN_2FA      => 'JBSWY3DPEHPK3PXP',
            %{ $args{env_file} || {} },
        }
    ) if $args{with_env};

    return ( $manager, $home, $state );
}

{
    my $home = tempdir( CLEANUP => 1 );
    my $manager = OpenVPN::Manager->new( home => $home, env => {} );
    is( $manager->{system}->('true'), 0, 'default system helper returns a zero exit code for true' );
    ok( $manager->{now}->() > 0, 'default now helper returns an epoch value' );
    is( $manager->{killer}->( 0, $$ ), 1, 'default killer helper can probe the current process' );
    $manager->{sleep}->(0);
    pass('default sleep helper runs without error');
}

{
    my $stdin_data = '';
    my $stdout = q{};
    my $stderr = q{};
    open my $stdin, '<', \$stdin_data or die $!;
    open my $out, '>', \$stdout or die $!;
    open my $err, '>', \$stderr or die $!;
    my ( $manager, $home ) = fake_manager(
        stdin_fh    => $stdin,
        stdout_fh   => $out,
        stderr_fh   => $err,
        with_env    => 0,
        openvpn_bin => '/fake/openvpn',
        system      => sub { return 0 },
    );
    my $exit = $manager->main_setup( '-u', 'bob', '-p', 'secret', '-2fa', '' );
    is( $exit, 0, 'main_setup exits zero on success' );
    like( $stdout, qr/"mode":"setup"/, 'main_setup prints JSON on success' );
    open my $fh, '<', File::Spec->catfile( $home, '.openvpn.env' ) or die $!;
    my $content = do { local $/; <$fh> };
    close $fh or die $!;
    unlike( $content, qr/OPENVPN_2FA=/, 'main_setup can clear an existing optional 2fa value with an empty argument' );
}

{
    my $stdin_data = "\n\n\n";
    my $stdout = q{};
    my $stderr = q{};
    open my $stdin, '<', \$stdin_data or die $!;
    open my $out, '>', \$stdout or die $!;
    open my $err, '>', \$stderr or die $!;
    my ( $manager ) = fake_manager(
        stdin_fh    => $stdin,
        stdout_fh   => $out,
        stderr_fh   => $err,
        with_env    => 0,
    );
    my $exit = $manager->main_setup;
    is( $exit, 2, 'main_setup returns two on validation failure' );
    like( $stderr, qr/OpenVPN username is required/, 'main_setup writes setup errors to stderr' );
}

{
    my $stdout = q{};
    my $stderr = q{};
    open my $out, '>', \$stdout or die $!;
    open my $err, '>', \$stderr or die $!;
    my ( $manager ) = fake_manager(
        stdout_fh => $out,
        stderr_fh => $err,
    );
    my $exit = $manager->main_setup('--bogus');
    is( $exit, 2, 'main_setup returns two for unsupported arguments' );
    is( $stdout, q{}, 'main_setup does not print JSON on argument errors' );
    like( $stderr, qr/Unsupported option: --bogus/, 'main_setup reports unsupported options on stderr' );
}

{
    my $stdout = q{};
    my $stderr = q{};
    open my $out, '>', \$stdout or die $!;
    open my $err, '>', \$stderr or die $!;
    my ( $manager ) = fake_manager(
        stdout_fh   => $out,
        stderr_fh   => $err,
        with_env    => 1,
        openvpn_bin => '/fake/openvpn',
    );
    my $exit = $manager->main_connect('--auto');
    is( $exit, 0, 'main_connect returns zero for a healthy connect' );
    like( $stdout, qr/"status":"connected"/, 'main_connect prints connect JSON' );
}

{
    my $stdout = q{};
    my $stderr = q{};
    open my $out, '>', \$stdout or die $!;
    open my $err, '>', \$stderr or die $!;
    my ( $manager ) = fake_manager(
        stdout_fh   => $out,
        stderr_fh   => $err,
        with_env    => 1,
        system      => sub { return 1 },
    );
    my $exit = $manager->main_connect('--auto');
    is( $exit, 1, 'main_connect returns nonzero when the connect attempt fails' );
    like( $stdout, qr/"status":"retrying"/, 'main_connect still prints JSON status for retrying failures' );
}

{
    my $stdout = q{};
    my $stderr = q{};
    open my $out, '>', \$stdout or die $!;
    open my $err, '>', \$stderr or die $!;
    my ( $manager ) = fake_manager(
        stdout_fh   => $out,
        stderr_fh   => $err,
        with_env    => 1,
    );
    my $exit = $manager->main_connect('--bogus');
    is( $exit, 2, 'main_connect returns two for unsupported arguments' );
    like( $stderr, qr/Unsupported option: --bogus/, 'main_connect reports unsupported options on stderr' );
}

{
    my ( $manager ) = fake_manager( with_env => 1 );
    my $result = $manager->execute_connect;
    is( $result->{status}, 'connected', 'plain connect performs a one-off connection' );
    is( $manager->read_state->{auto_reconnect}, 0, 'plain connect disables reconnect state after the one-off attempt' );
}

{
    my ( $manager ) = fake_manager( with_env => 1 );
    $manager->execute_connect('--auto');
    my $result = $manager->execute_connect('--collector');
    is( $result->{status}, 'connected', 'collector reports connected when the pid is already live' );
}

{
    my ( $manager ) = fake_manager( with_env => 1, linger_after_term => 1 );
    $manager->execute_connect('--auto');
    my $result = $manager->execute_disconnect;
    is( $result->{status}, 'disconnected', 'disconnect still reports disconnected when the process lingers briefly after TERM' );
}

{
    my ( $manager ) = fake_manager( with_env => 1 );
    my $result = $manager->handle_connect_failure(
        mode      => 'connect',
        auto      => 0,
        env       => $manager->read_env_file,
        state     => $manager->read_state,
        error_msg => 'manual failure',
    );
    is( $result->{status}, 'failed', 'manual connect failures stay as failed instead of retrying' );
}

{
    my ( $manager, $home ) = fake_manager( with_env => 1, profile_name => 'fallback.ovpn' );
    unlink File::Spec->catfile( $home, '.openvpn', 'fallback.ovpn' );
    my $custom = File::Spec->catfile( $home, '.openvpn', 'other.ovpn' );
    open my $fh, '>', $custom or die $!;
    print {$fh} "client\n";
    close $fh or die $!;
    is( $manager->find_openvpn_config, $custom, 'config discovery falls back to the first ovpn file in the directory' );
}

{
    my $home = tempdir( CLEANUP => 1 );
    my $pf   = File::Spec->catdir( $home, 'ProgramFiles' );
    my $cfgd = File::Spec->catdir( $pf, 'OpenVPN', 'config' );
    make_path($cfgd);
    my $cfg = File::Spec->catfile( $cfgd, 'other.ovpn' );
    open my $fh, '>', $cfg or die $!;
    print {$fh} "client\n";
    close $fh or die $!;
    my $manager = OpenVPN::Manager->new(
        home      => $home,
        env       => { ProgramFiles => $pf },
        osname    => 'MSWin32',
        interactive => 0,
    );
    is( $manager->find_openvpn_config, $cfg, 'Windows config discovery checks Program Files OpenVPN config paths' );
}

{
    my ( $manager ) = fake_manager( with_env => 1 );
    ok( $manager->log_file =~ /openvpn\.log\z/, 'log_file returns the managed OpenVPN log path' );
    ok( !$manager->pid_alive(0), 'pid_alive returns false for pid zero through the manager wrapper' );
}

{
    my ( $manager ) = fake_manager( with_env => 1 );
    my $uri_code = $manager->current_twofa_code('otpauth://totp/Example?secret=JBSWY3DPEHPK3PXP');
    like( $uri_code, qr/^\d{6}\z/, 'current_twofa_code accepts an otpauth URI secret' );
}

{
    my ( $manager ) = fake_manager( with_env => 1, openvpn_bin => undef, env => { OPENVPN_BIN => '/env/openvpn' } );
    is( $manager->openvpn_bin( {} ), '/env/openvpn', 'openvpn_bin prefers the process environment override when no constructor override exists' );
    my ( $manager2 ) = fake_manager( with_env => 1, openvpn_bin => undef );
    is( $manager2->openvpn_bin( { OPENVPN_BIN => '/file/openvpn' } ), '/file/openvpn', 'openvpn_bin falls back to the env file value' );
    my ( $manager3 ) = fake_manager( with_env => 1, openvpn_bin => undef, env => {} );
    is( $manager3->openvpn_bin( {} ), 'openvpn', 'openvpn_bin falls back to the plain binary name last' );
}

{
    my ( $manager ) = fake_manager( with_env => 1 );
    is( $manager->expand_tilde('/tmp/plain.ovpn'), '/tmp/plain.ovpn', 'expand_tilde leaves non-home paths untouched' );
}

{
    my $stdin_data = "super-secret\n";
    my $stdout = q{};
    my @system_calls;
    open my $stdin, '<', \$stdin_data or die $!;
    open my $out, '>', \$stdout or die $!;
    my ( $manager ) = fake_manager(
        stdin_fh    => $stdin,
        stdout_fh   => $out,
        interactive => 1,
        system      => sub {
            push @system_calls, [@_];
            return 0;
        },
    );
    my $value = $manager->prompt_hidden('OpenVPN secret: ');
    is( $value, 'super-secret', 'prompt_hidden returns the entered secret in interactive mode' );
    is_deeply(
        \@system_calls,
        [ [ 'stty', '-echo' ], [ 'stty', 'echo' ] ],
        'prompt_hidden toggles terminal echo around the hidden prompt'
    );
    is( $stdout, "OpenVPN secret: \n", 'prompt_hidden prints the prompt and a trailing newline' );
}

{
    my $stdin_data = "windows-secret\n";
    my $stdout = q{};
    my @system_calls;
    open my $stdin, '<', \$stdin_data or die $!;
    open my $out, '>', \$stdout or die $!;
    my ( $manager ) = fake_manager(
        stdin_fh    => $stdin,
        stdout_fh   => $out,
        interactive => 1,
        system      => sub {
            push @system_calls, [@_];
            return 0;
        },
        env    => {},
    );
    $manager->{osname} = 'MSWin32';
    my $value = $manager->prompt_hidden('Windows secret: ');
    is( $value, 'windows-secret', 'prompt_hidden falls back to a visible prompt on Windows' );
    is_deeply( \@system_calls, [], 'prompt_hidden does not invoke stty on Windows' );
    is( $stdout, 'Windows secret: ', 'Windows prompt_hidden uses the visible prompt output path' );
}

{
    my $stdout = q{};
    open my $out, '>', \$stdout or die $!;
    my ( $manager ) = fake_manager(
        stdout_fh => $out,
        with_env  => 1,
    );
    my $exit = $manager->main_noreconnect;
    is( $exit, 0, 'main_noreconnect exits zero' );
    like( $stdout, qr/"status":"reconnect_disabled"/, 'main_noreconnect prints JSON' );
}

{
    my $stdout = q{};
    my $stderr = q{};
    open my $out, '>', \$stdout or die $!;
    open my $err, '>', \$stderr or die $!;
    my ( $manager ) = fake_manager(
        stdout_fh => $out,
        stderr_fh => $err,
        with_env  => 1,
    );
    my $exit = $manager->main_noreconnect('--bogus');
    is( $exit, 2, 'main_noreconnect returns two for unsupported arguments' );
    is( $stdout, q{}, 'main_noreconnect does not print JSON on argument errors' );
    like( $stderr, qr/Unsupported option: --bogus/, 'main_noreconnect reports unsupported options on stderr' );
}

{
    my $stdout = q{};
    open my $out, '>', \$stdout or die $!;
    my ( $manager ) = fake_manager(
        stdout_fh => $out,
        with_env  => 1,
    );
    $manager->execute_connect('--auto');
    my $exit = $manager->main_disconnect;
    is( $exit, 0, 'main_disconnect exits zero' );
    like( $stdout, qr/"status":"disconnected"/, 'main_disconnect prints JSON' );
}

{
    my $stdout = q{};
    my $stderr = q{};
    open my $out, '>', \$stdout or die $!;
    open my $err, '>', \$stderr or die $!;
    my ( $manager ) = fake_manager(
        stdout_fh => $out,
        stderr_fh => $err,
        with_env  => 1,
    );
    my $exit = $manager->main_disconnect('--bogus');
    is( $exit, 2, 'main_disconnect returns two for unsupported arguments' );
    is( $stdout, q{}, 'main_disconnect does not print JSON on argument errors' );
    like( $stderr, qr/Unsupported option: --bogus/, 'main_disconnect reports unsupported options on stderr' );
}

done_testing;
