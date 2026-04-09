import base64
from Crypto.PublicKey import RSA
from Crypto.Cipher import PKCS1_OAEP
from Crypto.Hash import SHA256

def encrypt_device_id(device_id: str, public_key_pem: str) -> str:
    """
    Encrypts a device ID using an RSA public key and Base64-encodes the result.

    This mimics the client-side process needed to generate the
    'x-encrypted-device-id' header value.

    Args:
        device_id: The device ID string to encrypt.
        public_key_pem: The RSA public key in PEM format.

    Returns:
        The Base64-encoded encrypted device ID as a string.
    """
    try:
        # Import the RSA public key from the PEM string
        public_key = RSA.import_key(public_key_pem)

        # Create a cipher object using PKCS1_OAEP padding with SHA256.
        # This must match the decryption implementation on the Go backend.
        cipher_rsa = PKCS1_OAEP.new(public_key, hashAlgo=SHA256)

        # Encrypt the device ID (which must be converted to bytes)
        encrypted_device_id_bytes = cipher_rsa.encrypt(device_id.encode('utf-8'))

        # Base64-encode the resulting encrypted bytes to get a string
        encoded_encrypted_device_id = base64.b64encode(encrypted_device_id_bytes)

        return encoded_encrypted_device_id.decode('utf-8')

    except Exception as e:
        print(f"An error occurred during encryption: {e}")
        return ""

if __name__ == '__main__':
    # --- Example Usage ---

    # 1. Replace this with the actual public key for a family in your database.
    # It is stored in the `families` table and starts with '-----BEGIN PUBLIC KEY-----'.
    # The public key is Base64 encoded, so we decode it first.
    base64_public_key = "LS0tLS1CRUdJTiBQVUJMSUMgS0VZLS0tLS0KTUlJQklqQU5CZ2txaGtpRzl3MEJBUUVGQUFPQ0FROEFNSUlCQ2dLQ0FRRUF0Y2UwTW9wT3hoVFp2bFFTVEFxRAoxWHg1UFVKZ3dEK2VhbGJDWHZHVGZlSlpDR2hLSytNVlFtQlZMOWF4V1duYXR2bjY1aFQ4RDM5V0J3dzFlaXFnCjh5SVJmeTQxQmNvekJDMDA3anh6Wi9NZ1JxTHowbTdEUmhLUFJMQ1RsU2xvMFRwQlVwUlhBZkVDN0xJSVRkVGcKV25Cb0cxUUVXQVJGZ2FJVFhIRzk5NE91TFVtaFErY2UzTDd1WC9IOFVxcVA2QmNpMWZxMjhRM1hlb1Y2Ykg1dgpDeVQ2c1lFY1Zxa1VhRHI5aVcwQkQ3MUplMUZjWk1QSDNjQUlLRTUyRm0rS1ZLZ1VadGJYTkxwaUJVdGZSblpVCndyQ2I0bDA5WWt6UFRiVEFrNEZGbkJHaE56Tms0dHV6cWVzVW5DbGcwSk8rUW9oTXVxMzM3dUVFVmE1NDJlVGwKNXdJREFRQUIKLS0tLS1FTkQgUFVCTElDIEtFWS0tLS0tCg=="
    example_public_key = base64.b64decode(base64_public_key).decode('utf-8').strip()

    # 2. Use a device ID that is registered and active for a child in that family.
    example_device_id = "eyhy7474ndddsbddddsb"

    # 3. Run the script to get the encrypted value.
    encrypted_header_value = encrypt_device_id(example_device_id, example_public_key)

    if encrypted_header_value:
        print(f"Device ID to encrypt: {example_device_id}\n")
        print("--- Use this value for the 'x-encrypted-device-id' header ---")
        print(encrypted_header_value)