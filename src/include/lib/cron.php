<?php

class RbCron
{
    private static $fields = array(
        array('minute', 0, 59),
        array('hour', 0, 23),
        array('day of month', 1, 31),
        array('month', 1, 12),
        array('day of week', 0, 7),
    );

    private static $macros = array(
        '@hourly'   => '0 * * * *',
        '@daily'    => '0 0 * * *',
        '@midnight' => '0 0 * * *',
        '@weekly'   => '0 0 * * 0',
        '@monthly'  => '0 0 1 * *',
        '@yearly'   => '0 0 1 1 *',
        '@annually' => '0 0 1 1 *',
    );

    private static $names = array(
        3 => array('jan' => 1, 'feb' => 2, 'mar' => 3, 'apr' => 4, 'may' => 5, 'jun' => 6,
                   'jul' => 7, 'aug' => 8, 'sep' => 9, 'oct' => 10, 'nov' => 11, 'dec' => 12),
        4 => array('sun' => 0, 'mon' => 1, 'tue' => 2, 'wed' => 3, 'thu' => 4, 'fri' => 5, 'sat' => 6),
    );

    public $expr;

    private $sets = array();
    private $domStar = false;
    private $dowStar = false;

    public static function parse($expr)
    {
        $c = new self();
        $c->expr = trim((string)$expr);
        $text = strtolower($c->expr);
        if (isset(self::$macros[$text])) {
            $text = self::$macros[$text];
        }
        $parts = preg_split('/\s+/', $text);
        if ($text === '' || count($parts) !== 5) {
            throw new InvalidArgumentException(
                'A schedule needs five fields: minute, hour, day of month, month, day of week');
        }
        foreach ($parts as $i => $part) {
            $c->sets[$i] = self::parseField($part, $i);
        }
        if (isset($c->sets[4][7])) {
            unset($c->sets[4][7]);
            $c->sets[4][0] = true;
        }
        $c->domStar = $parts[2][0] === '*';
        $c->dowStar = $parts[4][0] === '*';
        return $c;
    }

    public static function problem($expr)
    {
        try {
            self::parse($expr);
            return '';
        } catch (InvalidArgumentException $e) {
            return $e->getMessage();
        }
    }

    private static function parseField($text, $index)
    {
        list($label, $min, $max) = self::$fields[$index];
        $set = array();
        foreach (explode(',', $text) as $item) {
            if ($item === '') {
                throw new InvalidArgumentException("$label: empty list entry");
            }
            $step = 1;
            if (strpos($item, '/') !== false) {
                list($item, $stepText) = explode('/', $item, 2);
                if (!ctype_digit($stepText) || (int)$stepText < 1) {
                    throw new InvalidArgumentException("$label: step \"$stepText\" must be a whole number of at least 1");
                }
                $step = (int)$stepText;
            }
            if ($item === '*') {
                $lo = $min;
                $hi = $max;
            } elseif (strpos($item, '-') !== false) {
                list($a, $b) = explode('-', $item, 2);
                $lo = self::value($a, $index);
                $hi = self::value($b, $index);
                if ($lo > $hi) {
                    throw new InvalidArgumentException("$label: range $item runs backwards");
                }
            } else {
                $lo = self::value($item, $index);
                $hi = $step > 1 ? $max : $lo;
            }
            for ($v = $lo; $v <= $hi; $v += $step) {
                $set[$v] = true;
            }
        }
        return $set;
    }

    private static function value($text, $index)
    {
        list($label, $min, $max) = self::$fields[$index];
        if (isset(self::$names[$index][$text])) {
            return self::$names[$index][$text];
        }
        if (!ctype_digit($text)) {
            throw new InvalidArgumentException("$label: \"$text\" is not a number" .
                ($index >= 3 ? ' or a name' : ''));
        }
        $v = (int)$text;
        if ($v < $min || $v > $max) {
            throw new InvalidArgumentException("$label: $v is outside $min-$max");
        }
        return $v;
    }

    private function dayMatches($y, $m, $d)
    {
        if (!isset($this->sets[3][$m])) {
            return false;
        }
        $dom = isset($this->sets[2][$d]);
        $dow = isset($this->sets[4][(int)date('w', mktime(12, 0, 0, $m, $d, $y))]);
        if ($this->domStar || $this->dowStar) {
            return $dom && $dow;
        }
        return $dom || $dow;
    }

    public function matches($ts)
    {
        $f = explode(' ', date('Y n j G i', $ts));
        return $this->dayMatches((int)$f[0], (int)$f[1], (int)$f[2])
            && isset($this->sets[1][(int)$f[3]])
            && isset($this->sets[0][(int)$f[4]]);
    }

    public function prev($ts)
    {
        list($y, $m, $d, $h, $i) = array_map('intval', explode(' ', date('Y n j G i', $ts)));
        for ($day = 0; $day < 366 * 28; $day++) {
            if ($this->dayMatches($y, $m, $d)) {
                for ($hh = $day === 0 ? $h : 23; $hh >= 0; $hh--) {
                    if (!isset($this->sets[1][$hh])) {
                        continue;
                    }
                    for ($mm = ($day === 0 && $hh === $h) ? $i : 59; $mm >= 0; $mm--) {
                        if (isset($this->sets[0][$mm])) {
                            $t = mktime($hh, $mm, 0, $m, $d, $y);
                            if ($t <= $ts) {
                                return $t;
                            }
                        }
                    }
                }
            }
            if (--$d < 1) {
                if (--$m < 1) {
                    $m = 12;
                    $y--;
                }
                $d = (int)date('t', mktime(12, 0, 0, $m, 1, $y));
            }
        }
        return null;
    }

    public function next($ts)
    {
        list($y, $m, $d, $h, $i) = array_map('intval', explode(' ', date('Y n j G i', $ts)));
        for ($day = 0; $day < 366 * 28; $day++) {
            if ($this->dayMatches($y, $m, $d)) {
                for ($hh = $day === 0 ? $h : 0; $hh <= 23; $hh++) {
                    if (!isset($this->sets[1][$hh])) {
                        continue;
                    }
                    for ($mm = ($day === 0 && $hh === $h) ? $i + 1 : 0; $mm <= 59; $mm++) {
                        if (isset($this->sets[0][$mm])) {
                            $t = mktime($hh, $mm, 0, $m, $d, $y);
                            if ($t > $ts) {
                                return $t;
                            }
                        }
                    }
                }
            }
            if (++$d > (int)date('t', mktime(12, 0, 0, $m, 1, $y))) {
                $d = 1;
                if (++$m > 12) {
                    $m = 1;
                    $y++;
                }
            }
        }
        return null;
    }

    public function upcoming($ts, $count)
    {
        $out = array();
        while (count($out) < $count && ($ts = $this->next($ts)) !== null) {
            $out[] = $ts;
        }
        return $out;
    }
}
