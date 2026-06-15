INSERT INTO tool_catalog (provider, id, display_name, tool_type, unit, price_per_unit) VALUES
    ('openai', 'web_search_standard',             'Web Search (all models)',            'web_search',  'call',    0.010000),
    ('openai', 'web_search_preview_reasoning',    'Web Search Preview (reasoning)',     'web_search',  'call',    0.010000),
    ('openai', 'web_search_preview_non_reasoning','Web Search Preview (non-reasoning)', 'web_search',  'call',    0.025000),
    ('openai', 'file_search',                     'File Search',                        'file_search', 'call',    0.002500),
    ('openai', 'container_1gb',                   'Container (1 GB)',                   'container',   'session', 0.030000),
    ('openai', 'container_4gb',                   'Container (4 GB)',                   'container',   'session', 0.120000),
    ('openai', 'container_16gb',                  'Container (16 GB)',                  'container',   'session', 0.480000),
    ('openai', 'container_64gb',                  'Container (64 GB)',                  'container',   'session', 1.920000),
    ('openai', 'function_calling',                'Function Calling',                   'function',    'call',    0.000000),
    ('openai', 'custom_tool',                     'Custom Tool',                        'custom',      'call',    0.000000),
    ('openai', 'mcp_tool',                        'MCP Tool',                           'mcp',         'call',    0.000000)
ON CONFLICT (provider, id) DO NOTHING;
