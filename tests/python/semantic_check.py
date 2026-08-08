from __future__ import annotations

import argparse
import base64
import json
import subprocess
import sys
from pathlib import Path


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--generated", type=Path, required=True)
    parser.add_argument("--python-generated", type=Path, required=True)
    parser.add_argument("--probe", type=Path, required=True)
    args = parser.parse_args()
    sys.path.insert(0, str(args.python_generated))

    import compatibility_options_pb2
    from compatibility_pb2 import Compatibility

    message = Compatibility()
    message.present = "present"
    message.text = "text"
    message.entries["key"].value = "value"
    message.observed_at.seconds = 123
    message.observed_at.nanos = 456
    assert message.HasField("present")
    assert message.WhichOneof("payload") == "text"
    assert message.entries["key"].value == "value"
    assert (message.observed_at.seconds, message.observed_at.nanos) == (123, 456)
    wire = message.SerializeToString(deterministic=True)

    unknown = wire + bytes((0xA0, 0x06, 0x07))
    parsed = Compatibility()
    parsed.ParseFromString(unknown)
    assert bytes((0xA0, 0x06, 0x07)) in parsed.SerializeToString(deterministic=True)

    method = compatibility_options_pb2.DESCRIPTOR.services_by_name[
        "OptionService"
    ].methods_by_name["Describe"]
    assert (
        method.GetOptions().Extensions[compatibility_options_pb2.compatibility_method]
        == "compatibility-v1"
    )

    probe = subprocess.check_output(
        ["go", "run", "./probe"], cwd=args.generated, text=True
    )
    facts = json.loads(probe)
    go_wire = base64.b64decode(facts["wire"])
    go_message = Compatibility()
    go_message.ParseFromString(go_wire)
    assert go_message.present == "present"
    assert go_message.text == "text"
    assert go_message.entries["key"].value == "value"
    assert (go_message.observed_at.seconds, go_message.observed_at.nanos) == (123, 456)
    reciprocal = json.loads(
        subprocess.check_output(
            ["go", "run", "./probe", base64.b64encode(unknown).decode()],
            cwd=args.generated,
            text=True,
        )
    )
    assert reciprocal["parsed_present"] == "present"
    assert reciprocal["parsed_text"] == "text"
    assert reciprocal["parsed_map_value"] == "value"
    assert (reciprocal["parsed_seconds"], reciprocal["parsed_nanos"]) == (123, 456)
    assert reciprocal["unknown_preserved"]
    expected_fields = {
        field.name: (
            field.number,
            field.type,
            field.containing_oneof is not None,
            field.message_type.full_name.endswith("EntriesEntry")
            if field.message_type
            else False,
            field.message_type.full_name.endswith("Timestamp")
            if field.message_type
            else False,
        )
        for field in Compatibility.DESCRIPTOR.fields
    }
    assert facts["fields"] == [
        "present:1:string:oneof",
        "entries:2:message:map",
        "kind:5:enum",
        "observed_at:6:message:timestamp",
        "text:3:string:oneof",
        "raw:4:bytes:oneof",
    ]
    for name, number, kind, oneof, is_map, is_timestamp in [
        ("present", 1, 9, True, False, False),
        ("entries", 2, 11, False, True, False),
        ("kind", 5, 14, False, False, False),
        ("text", 3, 9, True, False, False),
        ("raw", 4, 12, True, False, False),
        ("observed_at", 6, 11, False, False, True),
    ]:
        assert expected_fields[name] == (number, kind, oneof, is_map, is_timestamp)
    print(
        "semantic compatibility: wire, presence, oneof, map, Timestamp, custom option, unknown, descriptor facts, Go facts: ok"
    )


if __name__ == "__main__":
    main()
