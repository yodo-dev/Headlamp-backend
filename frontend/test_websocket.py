import websocket
import json
import threading
import time
import os
import random

# --- Configuration ---
# Replace with your actual values or set them as environment variables.
JWT_TOKEN = os.environ.get(
    "HEADLAMP_JWT_TOKEN",
    "eyJhbGciOiJSUzI1NiIsImtpZCI6IjM4MDI5MzRmZTBlZWM0NmE1ZWQwMDA2ZDE0YTFiYWIwMWUzNDUwODMiLCJ0eXAiOiJKV1QifQ.eyJkZXZpY2VfaWQiOiIiLCJmYW1pbHlfaWQiOiJhM2UyNzgwNS03YTY4LTRlNzktOGU4Ny0yMTJkMzNhYWIwMWEiLCJyb2xlIjoicGFyZW50IiwidXNlcl9pZCI6Ijk1Njc5MDZjLWJlYzctNDBhOC1hNjYzLTQ1NDA3MGFkNWJiNSIsImlzcyI6Imh0dHBzOi8vc2VjdXJldG9rZW4uZ29vZ2xlLmNvbS9oZWFkbGFtcC1kZXYtOWQwMDYiLCJhdWQiOiJoZWFkbGFtcC1kZXYtOWQwMDYiLCJhdXRoX3RpbWUiOjE3NjMxMjkxMzcsInN1YiI6Ijk1Njc5MDZjLWJlYzctNDBhOC1hNjYzLTQ1NDA3MGFkNWJiNSIsImlhdCI6MTc2MzEyOTEzNywiZXhwIjoxNzYzMTMyNzM3LCJmaXJlYmFzZSI6eyJpZGVudGl0aWVzIjp7fSwic2lnbl9pbl9wcm92aWRlciI6ImN1c3RvbSJ9fQ.hzW6jGgqjOllX6Xfk7sJaOJt0XlT0132v7AyB2zbCPKmhnG3_ttsgVqfEWjAryWoFU0SLEDoc8AZiaQcqMBwclOVNNikd1D548rYbmv_eRC0EcU63KVx6YAxYWmpccT9CdB1cPG8Jx8h6N7CSxZZYg91suAxV6z7ZvIDpsdcWU9QSI68_QqpGOuzHkHnE1i_8MXhkWA-1f5D6pG1raEyClaKgBCSuXlCb1P32Ae105xqS67cdSBATiLH-VmG9EamoJ1qy9qMh6x8ssmrQkcbMzZHbLGnSMNybUCdIkgt0F0sSKGhbc3MDhifibnGaQ7rCofsBMmdyGy2E_oUuXtQ2Q",
)
CHILD_ID = os.environ.get("HEADLAMP_CHILD_ID", "4b6185bb-7524-4acd-a4fd-c1eb7c707422")
WS_URL = f"ws://localhost:8080/v1/parent/child/{CHILD_ID}/digital-permit-test/v2/ws"
# ---------------------

# Auto-answer configuration
AUTO_ANSWER = True
ANSWER_DELAY = 0.5  # Delay between auto-answers in seconds
question_count = 0
test_complete = False


def on_message(ws, message):
    """Handles incoming messages from the server."""
    global question_count, test_complete
    
    print("\n<<< Received from server:")
    try:
        data = json.loads(message)
        print(json.dumps(data, indent=2))

        # Check if test is complete
        if data.get("status") == "complete":
            print("\n--- Test finished. Closing connection. ---")
            test_complete = True
            time.sleep(1)
            ws.close()
            return

        # Auto-answer if enabled and we get a question
        if AUTO_ANSWER and data.get("status") == "question":
            question_count += 1
            print(f"\n[AUTO-ANSWER #{question_count}]")
            
            # Generate random answer based on question type
            question_type = data.get("question_type", "")
            
            if question_type == "true_false":
                answer = random.choice(["True", "False"])
            elif question_type == "multiple_choice":
                answer = random.choice(["A", "B", "C", "D"])
            else:
                # For open-ended, fill_in_blank, scenario
                answer = random.choice(["Yes", "No", "Maybe", "I think so", "Not sure"])
            
            print(f"Sending answer: {answer}")
            
            # Wait a bit before sending
            time.sleep(ANSWER_DELAY)
            
            message_obj = {"role": "user", "answer": answer}
            ws.send(json.dumps(message_obj))
            print(f">>> Sent to server: {json.dumps(message_obj)}")

    except json.JSONDecodeError:
        print(message)


def on_error(ws, error):
    """Handles WebSocket errors."""
    print(f"\n--- WebSocket Error: {error} ---")


def on_close(ws, close_status_code, close_msg):
    """Handles WebSocket connection closure."""
    print("\n--- WebSocket Connection Closed ---")


def on_open(ws):
    """Handles actions to take once the WebSocket connection is open."""
    print("--- WebSocket Connection Opened ---")
    print("Auto-answering enabled. Waiting for questions...")
    
    if not AUTO_ANSWER:
        def run(*args):
            while ws.sock and ws.sock.connected:
                try:
                    answer_text = input(">>> Enter your answer (or 'quit' to exit): ")

                    if answer_text.lower() == "quit":
                        break

                    message = {"role": "user", "answer": answer_text}
                    ws.send(json.dumps(message))
                    print(f"\n>>> Sent to server: {json.dumps(message)}")

                except (KeyboardInterrupt, EOFError):
                    break
                except Exception as e:
                    print(f"An error occurred while sending: {e}")
                    break
            # Ensure the connection is closed if the loop exits
            if ws.sock and ws.sock.connected:
                ws.close()

        thread = threading.Thread(target=run)
        thread.daemon = True
        thread.start()


if __name__ == "__main__":
    if JWT_TOKEN == "your_jwt_token_here" or CHILD_ID == "your_child_id_here":
        print("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
        print("!!! PLEASE SET your JWT_TOKEN and CHILD_ID in the script or       !!!")
        print(
            "!!! as environment variables (HEADLAMP_JWT_TOKEN, HEADLAMP_CHILD_ID) !!!"
        )
        print("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
    else:
        print(f"Connecting to {WS_URL}...")
        # websocket.enableTrace(True) # Uncomment for verbose logging
        ws = websocket.WebSocketApp(
            WS_URL,
            header={"Authorization": f"Bearer {JWT_TOKEN}"},
            on_open=on_open,
            on_message=on_message,
            on_error=on_error,
            on_close=on_close,
        )
        try:
            print(f"AUTO_ANSWER: {AUTO_ANSWER}")
            print(f"ANSWER_DELAY: {ANSWER_DELAY}s")
            print("Starting test...\n")
            ws.run_forever()
        except KeyboardInterrupt:
            print("\n--- Script interrupted by user. Closing connection. ---")
            ws.close()
        finally:
            print(f"\n=== Test Summary ===")
            print(f"Total questions answered: {question_count}")
            print(f"Test completed: {test_complete}")
