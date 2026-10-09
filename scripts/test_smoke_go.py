"""Offline harness checks; no core or Agent process is started."""
import socket
import unittest
from unittest.mock import Mock, patch

import smoke_go


class SmokeSafety(unittest.TestCase):
    def test_busy_port_aborts_before_writes_or_launches(self):
        with patch.object(socket.socket, "bind", side_effect=OSError("busy")), \
                patch.object(smoke_go, "launch") as launch:
            with self.assertRaisesRegex(RuntimeError, "no services were started"):
                smoke_go.run(None)
            launch.assert_not_called()

    def test_exited_process_is_not_an_existing_service(self):
        process = Mock()
        process.poll.return_value = 1
        with patch.object(smoke_go, "get_json") as get_json:
            with self.assertRaisesRegex(RuntimeError, "exited before readiness"):
                smoke_go.wait_ready("http://127.0.0.1:1", process=process)
            get_json.assert_not_called()


if __name__ == "__main__":
    unittest.main()
