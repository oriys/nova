<?php
// Example PHP function for Nova serverless platform

function handler($event, $context) {
    $name = $event['name'] ?? 'Anonymous';
    if (!is_string($name) || $name === '') {
        $name = 'Anonymous';
    }
    return [
        'message' => "Hello, {$name}!",
        'runtime' => 'php',
        'request_id' => $context['request_id'] ?? '',
    ];
}
