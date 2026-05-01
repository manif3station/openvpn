package OpenVPN::TOTP;

use strict;
use warnings;

use Digest::SHA qw(hmac_sha1);

sub new {
    my ( $class, %args ) = @_;
    return bless {
        now => $args{now} || sub { return time },
    }, $class;
}

sub code_for {
    my ( $self, $token ) = @_;
    return q{} if !defined $token || $token eq q{};
    return $token if $token =~ /^\d{6}\z/;

    my $secret = $self->extract_secret($token);
    my $bytes  = $self->decode_base32($secret);
    my $step   = int( $self->{now}->() / 30 );
    my $msg    = pack( 'N2', 0, $step );
    my $hmac   = hmac_sha1( $msg, $bytes );

    my $offset = ord( substr( $hmac, -1, 1 ) ) & 0x0f;
    my $slice  = substr( $hmac, $offset, 4 );
    my $value  = unpack( 'N', $slice ) & 0x7fffffff;

    return sprintf '%06d', $value % 1_000_000;
}

sub extract_secret {
    my ( $self, $token ) = @_;
    my $secret = defined $token ? $token : q{};

    if ( $secret =~ /secret=([^&]+)/i ) {
        $secret = $1;
    }

    $secret = $self->url_unescape($secret);
    $secret =~ s/[^A-Za-z2-7]//g;
    return uc $secret;
}

sub decode_base32 {
    my ( $self, $text ) = @_;
    my %alphabet;
    @alphabet{ qw(A B C D E F G H I J K L M N O P Q R S T U V W X Y Z 2 3 4 5 6 7) } = ( 0 .. 31 );

    my $buffer = 0;
    my $bits   = 0;
    my $out    = q{};

    for my $char ( split //, $text || q{} ) {
        die "Invalid OpenVPN 2FA token secret\n" if !exists $alphabet{$char};
        $buffer = ( $buffer << 5 ) | $alphabet{$char};
        $bits += 5;
        while ( $bits >= 8 ) {
            $bits -= 8;
            $out .= chr( ( $buffer >> $bits ) & 0xff );
        }
    }

    return $out;
}

sub url_unescape {
    my ( $self, $text ) = @_;
    $text = q{} if !defined $text;
    $text =~ s/\+/ /g;
    $text =~ s/%([0-9A-Fa-f]{2})/chr(hex($1))/eg;
    return $text;
}

1;
