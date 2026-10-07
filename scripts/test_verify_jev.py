"""Small offline boundary checks for the experiment client, not product tests."""
import http.server
import threading
import time
import unittest

from verify_jev import Client, accept, state_for, validate


class ResponseHandler(http.server.BaseHTTPRequestHandler):
    mode = "error"

    def do_CONNECT(self):
        if self.mode == "slow":
            time.sleep(.4)
        self.send_response(503 if self.mode != "redirect" else 302)
        self.send_header("Location", "https://example.invalid")
        self.end_headers()

    def log_message(self, *args):
        pass


class Boundaries(unittest.TestCase):
    def test_evidence_veto(self):
        for kind, choice, expected in [("missing", "DIRECT", "UNCERTAIN"), ("conflict", "PROXY", "UNCERTAIN"),
                                      ("direct", "DIRECT", "DIRECT"), ("proxy", "PROXY", "PROXY")]:
            with self.subTest(kind=kind):
                answer = {"choice": choice, "probabilities": {choice: .99}}
                self.assertEqual(accept(state_for(kind), answer), expected)
        self.assertEqual(accept(state_for("direct"), {"choice": "DIRECT", "probabilities": {"DIRECT": .6}}), "UNCERTAIN")

    def test_invalid_answers(self):
        for scores in [{}, {"DIRECT": float("nan"), "PROXY": 0, "UNCERTAIN": 0},
                       {"DIRECT": 1, "PROXY": 1, "UNCERTAIN": 1},
                       {"DIRECT": True, "PROXY": 0, "UNCERTAIN": 0}]:
            with self.subTest(scores=scores), self.assertRaises(ValueError):
                validate({"answers": {"route": {"type": "choice", "choice": "DIRECT", "probabilities": scores}}})

    def test_proxy_failure_redirect_and_timeout(self):
        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), ResponseHandler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            for mode in ["error", "redirect", "slow"]:
                ResponseHandler.mode = mode
                client = Client("synthetic-not-a-real-key", f"http://127.0.0.1:{server.server_port}", timeout=.1)
                start = time.perf_counter()
                result = client.call(state_for())
                client.close()
                with self.subTest(mode=mode):
                    self.assertFalse(result["ok"])
                    self.assertNotIn("accepted", result)
                    self.assertNotIn("synthetic-not-a-real-key", str(result))
                    self.assertLess(time.perf_counter() - start, .6)
        finally:
            server.shutdown()
            server.server_close()


if __name__ == "__main__":
    unittest.main()
