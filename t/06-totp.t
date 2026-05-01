#!/usr/bin/env perl
use strict;
use warnings;

use Test::More;

use lib 'lib';
use OpenVPN::TOTP;

{
    my $totp = OpenVPN::TOTP->new( now => sub { return 1_700_000_000 } );
    is( $totp->code_for('123456'), '123456', 'six-digit token is treated as a static suffix' );
}

{
    my $totp = OpenVPN::TOTP->new;
    like( $totp->code_for('JBSWY3DPEHPK3PXP'), qr/^\d{6}\z/, 'default clock path also produces a six-digit TOTP value' );
}

{
    my $totp = OpenVPN::TOTP->new( now => sub { return 1_700_000_000 } );
    like( $totp->code_for('JBSWY3DPEHPK3PXP'), qr/^\d{6}\z/, 'base32 secret produces a six-digit TOTP value' );
}

{
    my $totp = OpenVPN::TOTP->new( now => sub { return 1_700_000_000 } );
    is(
        $totp->code_for('otpauth://totp/Example?secret=JBSWY3DPEHPK3PXP'),
        $totp->code_for('JBSWY3DPEHPK3PXP'),
        'otpauth URI secrets resolve to the same TOTP value as the raw base32 secret'
    );
}

{
    my $totp = OpenVPN::TOTP->new( now => sub { return 1_700_000_000 } );
    is(
        $totp->extract_secret('otpauth://totp/Example?secret=JBSW%59 3DPEHPK3PXP&issuer=DD'),
        'JBSWY3DPEHPK3PXP',
        'extract_secret decodes escaped characters and strips non-base32 separators'
    );
}

{
    my $totp = OpenVPN::TOTP->new;
    my $ok = eval { $totp->decode_base32('BAD!'); 1 };
    ok( !$ok, 'invalid base32 input is rejected' );
    like( $@, qr/Invalid OpenVPN 2FA token secret/, 'invalid base32 input reports a clear error' );
}

done_testing;
