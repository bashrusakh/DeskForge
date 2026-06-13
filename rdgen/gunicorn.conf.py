import os

# Adjust these values as needed
bind = "0.0.0.0:8000"  # Host and port for Gunicorn to listen on
workers = 5  # The number of worker processes for concurrency (adjust based on system resources)
threads = 6
activate_base = True  # Activate your virtual environment if applicable

# Path to your Django project's main WSGI application file (usually manage.py)
wsgi_app = "rdgen.wsgi.application"