FROM python:3.12-slim
WORKDIR /srv
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
RUN apt-get update && apt-get install -y --no-install-recommends curl && rm -rf /var/lib/apt/lists/* \
    && useradd -m -u 10001 app
COPY pyproject.toml ./
COPY app ./app
COPY data ./data
RUN pip install --no-cache-dir .
USER app
EXPOSE 8090
HEALTHCHECK --interval=5s --timeout=3s --retries=10 CMD curl -fsS http://localhost:8090/health || exit 1
CMD ["python", "-m", "app.main"]
