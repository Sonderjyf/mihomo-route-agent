"""Offline node-format check: explicit file or stdin, no network or writes."""
import json
import sys

# Import the actual acceptance validator without creating a bytecode file.
sys.dont_write_bytecode = True
import real_acceptance as acceptance


def result(error="none", line=None, column=None, fields=(), expected=""):
    return {"valid": error == "none", "error": error, "line": line,
            "column": column, "fields": list(fields), "expected_type": expected}


def validate(raw):
    try:
        acceptance.node_input(raw)
    except acceptance.Refused as error:
        report = acceptance.Report()
        report.failure(error)
        diagnostic = report.payload()["diagnostic"]
        line = column = None
        if diagnostic["reason"] == "invalid_json":
            # The runtime intentionally discards JSONDecodeError.doc/message.
            # A second local parse obtains coordinates only; never serialize
            # the exception, its document, or a source-context excerpt.
            try:
                json.loads(raw)
            except json.JSONDecodeError as parse_error:
                line, column = parse_error.lineno, parse_error.colno
            except (ValueError, RecursionError):
                pass
        return result(diagnostic["reason"], line, column,
                      diagnostic["fields"], diagnostic["expected_type"])
    return result()


def main(argv=None):
    args = sys.argv[1:] if argv is None else argv
    try:
        if args == ["--help"]:
            print("Usage: python -B -X utf8 scripts/validate_vless_node.py [--file PATH]")
            print("Without --file, read UTF-8 JSON from stdin. No network or input storage.")
            return 0
        if args and (len(args) != 2 or args[0] != "--file"):
            output = result("invalid_arguments")
        else:
            # Match the runtime's character limit; never read an unbounded file.
            if args:
                with open(args[1], "r", encoding="utf-8", newline="") as source:
                    raw = source.read(16385)
            else:
                raw = sys.stdin.read(16385)
            output = validate(raw)
    except UnicodeError:
        output = result("invalid_encoding")
    except OSError:
        output = result("io_error")
    except KeyboardInterrupt:
        output = result("interrupted")
    except Exception:
        output = result("unexpected_error")
    # No argv, path, raw input, unknown field names, or exception text in output.
    print(json.dumps(output, ensure_ascii=True, separators=(",", ":")))
    return 0 if output["valid"] else 1


if __name__ == "__main__":
    sys.exit(main())
