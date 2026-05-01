#!/usr/bin/env perl
use strict;
use warnings;

use File::Spec;
use File::Temp qw(tempdir);
use Test::More;

use lib 'lib';
use OpenVPN::Manager;

{
    my $home = tempdir( CLEANUP => 1 );
    my $stdin_data = "alice\nsecret-pass\nTOTPSECRET\n";
    open my $stdin,  '<', \$stdin_data or die $!;
    my $stdout = q{};
    my $stderr = q{};
    open my $out, '>', \$stdout or die $!;
    open my $err, '>', \$stderr or die $!;

    my $manager = OpenVPN::Manager->new(
        home        => $home,
        stdin_fh    => $stdin,
        stdout_fh   => $out,
        stderr_fh   => $err,
        interactive => 0,
        system      => sub { return 0 },
    );
    my $result = $manager->execute_setup;
    is( $result->{username}, 'alice', 'prompted setup stores username' );
    is( $result->{two_factor}, 'totp', 'prompted setup marks non-numeric 2fa as totp' );
    ok( -f File::Spec->catfile( $home, '.openvpn.env' ), 'setup writes ~/.openvpn.env' );

    open my $fh, '<', File::Spec->catfile( $home, '.openvpn.env' ) or die $!;
    my $content = do { local $/; <$fh> };
    close $fh or die $!;
    like( $content, qr/^OPENVPN_USERNAME=alice$/m, 'env file stores username' );
    like( $content, qr/^OPENVPN_PASSWORD=secret-pass$/m, 'env file stores password' );
    like( $content, qr/^OPENVPN_2FA=TOTPSECRET$/m, 'env file stores 2fa token' );
}

{
    my $home = tempdir( CLEANUP => 1 );
    my $stdout = q{};
    open my $out, '>', \$stdout or die $!;
    my $manager = OpenVPN::Manager->new(
        home        => $home,
        stdout_fh   => $out,
        interactive => 0,
        system      => sub { return 0 },
    );
    my $result = $manager->execute_setup( '-u', 'bob', '-p', 's3cret', '-2fa', '123456' );
    is( $result->{username}, 'bob', 'argument-driven setup stores username' );
    is( $result->{two_factor}, 'static', 'six digit 2fa is marked as static' );
}

{
    my $home = tempdir( CLEANUP => 1 );
    my $stdin_data = "only-user\n\n";
    open my $stdin, '<', \$stdin_data or die $!;
    my $stdout = q{};
    my $stderr = q{};
    open my $out, '>', \$stdout or die $!;
    open my $err, '>', \$stderr or die $!;
    my $manager = OpenVPN::Manager->new(
        home        => $home,
        stdin_fh    => $stdin,
        stdout_fh   => $out,
        stderr_fh   => $err,
        interactive => 0,
        system      => sub { return 0 },
    );
    my $ok = eval { $manager->execute_setup; 1 };
    ok( !$ok, 'setup rejects missing password' );
    like( $@, qr/^OpenVPN password is required/, 'setup reports missing password clearly' );
}

done_testing;
