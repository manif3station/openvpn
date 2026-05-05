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

{
    my $home = tempdir( CLEANUP => 1 );
    my $manager = OpenVPN::Manager->new( home => $home, interactive => 0, system => sub { return 0 } );
    is( $manager->expand_tilde('~/.openvpn/config.ovpn'), File::Spec->catfile( $home, '.openvpn', 'config.ovpn' ), 'expand_tilde maps home paths' );
    is( $manager->twofa_mode('123456'), 'static', 'static token mode is detected' );
    is( $manager->twofa_mode('JBSWY3DPEHPK3PXP'), 'totp', 'non-numeric token mode is totp' );
}

{
    my $home = tempdir( CLEANUP => 1 );
    make_path( File::Spec->catdir( $home, 'openvpn', 'config' ) );
    my $preferred = File::Spec->catfile( $home, 'openvpn', 'config', 'client.ovpn' );
    open my $pfh, '>', $preferred or die $!;
    print {$pfh} "client\n";
    close $pfh or die $!;
    make_path( File::Spec->catdir( $home, '.config', 'openvpn' ) );
    my $ovpn = File::Spec->catfile( $home, '.config', 'openvpn', 'client.ovpn' );
    open my $fh, '>', $ovpn or die $!;
    print {$fh} "client\n";
    close $fh or die $!;
    my $manager = OpenVPN::Manager->new( home => $home, interactive => 0, system => sub { return 0 } );
    is( $manager->find_openvpn_config, $preferred, 'config finder discovers the ~/openvpn/config layout before the older fallback paths' );
}

{
    my $home = tempdir( CLEANUP => 1 );
    my $manager = OpenVPN::Manager->new(
        home        => $home,
        interactive => 0,
        system      => sub { return 0 },
        now         => sub { return 59 },
    );
    my $code = $manager->current_twofa_code('JBSWY3DPEHPK3PXP');
    like( $code, qr/^\d{6}\z/, 'totp generator returns a six digit code' );
    is( $manager->current_twofa_code('654321'), '654321', 'static six digit code is passed through unchanged' );
}

{
    my $home = tempdir( CLEANUP => 1 );
    my $manager = OpenVPN::Manager->new( home => $home, interactive => 0, system => sub { return 0 } );
    my $result = $manager->status_result(
        mode           => 'collector',
        status         => 'connected',
        status_icon    => '+',
        connected      => JSON::PP::true,
        auto_reconnect => JSON::PP::true,
        retry_count    => 0,
    );
    is( $manager->result_exit_code($result), 0, 'connected results map to zero exit' );
    $result->{connected} = JSON::PP::false;
    is( $manager->result_exit_code($result), 1, 'disconnected results map to nonzero exit' );
}

{
    my $home = tempdir( CLEANUP => 1 );
    my $manager = OpenVPN::Manager->new( home => $home, osname => 'MSWin32', interactive => 0, system => sub { return 0 } );
    is(
        $manager->run_dir,
        File::Spec->catdir( $home, 'openvpn', 'config', 'dd-runtime' ),
        'manager uses the Windows-oriented runtime directory under ~/openvpn/config'
    );
}

done_testing;
