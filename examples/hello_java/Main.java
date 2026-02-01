// Example Java function for Nova serverless platform
// Usage (inside VM): java -jar /code/handler /tmp/input.json
//
// This keeps dependencies at zero by using a tiny JSON field extractor
// sufficient for {"name":"..."} demo payloads.

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

public final class Main {
  private static final Pattern NAME_PATTERN =
      Pattern.compile("\"name\"\\s*:\\s*\"([^\"]*)\"");

  private static String escapeJson(String s) {
    StringBuilder out = new StringBuilder(s.length() + 8);
    for (int i = 0; i < s.length(); i++) {
      char c = s.charAt(i);
      switch (c) {
        case '\\\\':
          out.append("\\\\");
          break;
        case '"':
          out.append("\\\"");
          break;
        case '\n':
          out.append("\\n");
          break;
        case '\r':
          out.append("\\r");
          break;
        case '\t':
          out.append("\\t");
          break;
        default:
          out.append(c);
      }
    }
    return out.toString();
  }

  private static String extractName(String json) {
    Matcher m = NAME_PATTERN.matcher(json);
    if (!m.find()) return "Anonymous";
    String name = m.group(1);
    if (name == null || name.isEmpty()) return "Anonymous";
    return name;
  }

  public static void main(String[] args) throws IOException {
    String inputFile = args.length > 0 ? args[0] : "/tmp/input.json";
    String json = "";
    try {
      json = Files.readString(Path.of(inputFile), StandardCharsets.UTF_8);
    } catch (IOException ignored) {
      json = "";
    }

    String name = extractName(json);
    String message = "Hello, " + name + "!";

    String out =
        "{\"message\":\"" + escapeJson(message) + "\",\"runtime\":\"java\"}";
    System.out.print(out);
  }
}

