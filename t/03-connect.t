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

sub manager_for {
    my (%args) = @_;
    my $home = tempdir( CLEANUP => 1 );
    make_path( File::Spec->catdir( $home, '.openvpn' ) );
    my $ovpn = File::Spec->catfile( $home, '.openvpn', 'config.ovpn' );
    open my $cfh, '>', $ovpn or die $!;
    print {$cfh} "client\n";
    close $cfh or die $!;

    my $state = {
        pids => {},
        next_pid => 1000,
    };
    my $manager = OpenVPN::Manager->new(
        home        => $home,
        interactive => 0,
        openvpn_bin => '/fake/openvpn',
        system      => sub {
            my (@cmd) = @_;
            return $args{system_rc} if defined $args{system_rc};
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
            delete $state->{pids}{$pid};
            return 1;
        },
        now => sub { return 1_700_000_000 },
        sleep => sub { return 1 },
    );
    $manager->write_env_file(
        {
            OPENVPN_USERNAME => 'alice',
            OPENVPN_PASSWORD => 'secret',
            OPENVPN_2FA      => 'JBSWY3DPEHPK3PXP',
        }
    ) if $args{with_env};
    return ( $manager, $home, $state );
}

{
    my ( $manager ) = manager_for();
    my $result = $manager->execute_connect('--collector');
    is( $result->{status}, 'not_setup', 'collector reports setup-needed state before env file exists' );
    is( $manager->result_exit_code($result), 1, 'setup-needed collector state exits nonzero' );
    is( $result->{status_icon}, '?', 'setup-needed collector uses question mark indicator' );
}

{
    my ( $manager, $home ) = manager_for( with_env => 1 );
    my $result = $manager->execute_connect('--auto');
    is( $result->{status}, 'connected', 'manual auto connect reports connected' );
    is( $manager->read_state->{auto_reconnect}, 1, 'manual auto connect enables reconnect state' );
    ok( -f $manager->auth_file, 'manual connect writes auth file' );
    open my $fh, '<', $manager->auth_file or die $!;
    my @lines = <$fh>;
    close $fh or die $!;
    chomp @lines;
    is( $lines[0], 'alice', 'auth file keeps username on the first line' );
    like( $lines[1], qr/^secret\d{6}\z/, 'auth file appends a generated six digit code to the password' );
}

{
    my ( $manager ) = manager_for( with_env => 1 );
    my $result = $manager->execute_connect('--collector');
    is( $result->{status}, 'reconnected', 'collector reconnects when env exists and vpn is down' );
    is( $manager->result_exit_code($result), 0, 'successful collector reconnect exits zero' );
}

{
    my ( $manager ) = manager_for( with_env => 1, system_rc => 1 );
    my $result = $manager->execute_connect('--collector');
    is( $result->{status}, 'retrying', 'collector starts retry tracking after one failed reconnect' );
    is( $result->{retry_count}, 1, 'collector increments retry count after failure' );
    is( $manager->result_exit_code($result), 1, 'failed collector reconnect exits nonzero' );
}

{
    my ( $manager ) = manager_for( with_env => 1, system_rc => 1 );
    $manager->write_state(
        {
            auto_reconnect => JSON::PP::true,
            retry_count    => 4,
            status         => 'retrying',
        }
    );
    my $result = $manager->execute_connect('--collector');
    is( $result->{status}, 'reconnect_disabled', 'collector disables reconnect after five failures' );
    is( $manager->read_state->{auto_reconnect}, 0, 'collector turns auto reconnect off after exhaustion' );
}

{
    my ( $manager ) = manager_for( with_env => 1 );
    $manager->write_state(
        {
            auto_reconnect => JSON::PP::false,
            retry_count    => 2,
            status         => 'reconnect_disabled',
        }
    );
    my $result = $manager->execute_connect('--collector');
    is( $result->{status}, 'reconnect_disabled', 'collector stays disconnected when reconnect is disabled' );
}

{
    my ( $manager ) = manager_for( with_env => 1 );
    $manager->execute_connect('--auto');
    my $result = $manager->execute_disconnect;
    is( $result->{status}, 'disconnected', 'disconnect reports disconnected' );
    is( $manager->read_state->{auto_reconnect}, 0, 'disconnect disables reconnect state' );
}

{
    my ( $manager ) = manager_for( with_env => 1 );
    $manager->write_state(
        {
            auto_reconnect => JSON::PP::true,
            retry_count    => 2,
            status         => 'connected',
        }
    );
    my $result = $manager->execute_noreconnect;
    is( $result->{status}, 'reconnect_disabled', 'noreconnect reports reconnect disabled' );
    is( $manager->read_state->{auto_reconnect}, 0, 'noreconnect turns auto reconnect off' );
}

done_testing;
