using System;
using System.IO;
using System.Text.Json;

string inputFile = args.Length > 0 ? args[0] : "/tmp/input.json";
string json = "{}";

try
{
    json = File.ReadAllText(inputFile);
}
catch
{
    json = "{}";
}

string name = "Anonymous";
try
{
    using JsonDocument doc = JsonDocument.Parse(json);
    if (doc.RootElement.ValueKind == JsonValueKind.Object &&
        doc.RootElement.TryGetProperty("name", out JsonElement n) &&
        n.ValueKind == JsonValueKind.String)
    {
        string? s = n.GetString();
        if (!string.IsNullOrEmpty(s))
        {
            name = s;
        }
    }
}
catch
{
    name = "Anonymous";
}

string output = JsonSerializer.Serialize(new
{
    message = $"Hello, {name}!",
    runtime = "dotnet",
});

Console.Write(output);

