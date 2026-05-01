use strict;
use warnings;

use Test::More;

use lib 'lib';

BEGIN {
    use_ok('OpenVPN::Launcher');
    use_ok('OpenVPN::Manager');
    use_ok('OpenVPN::TOTP');
}

done_testing;
