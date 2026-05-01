use strict;
use warnings;

use JSON::PP qw(decode_json);
use Test::More;

open my $fh, '<', 'config/config.json' or die $!;
local $/;
my $config = decode_json(<$fh>);
close $fh or die $!;

is( ref $config->{collectors}, 'ARRAY', 'collector list exists' );
is( $config->{collectors}[0]{name}, 'connector', 'connector collector is declared' );
is( $config->{collectors}[0]{command}, 'dashboard openvpn.connect --collector', 'collector runs connect in collector mode' );
is( $config->{collectors}[0]{interval}, 10, 'collector interval is ten seconds' );
is( $config->{collectors}[0]{indicator}{icon}, 'OVPN[% status_icon %]', 'collector indicator uses status icon template' );

done_testing;
