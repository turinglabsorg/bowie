FROM python:3.12-slim
RUN apt-get update && apt-get install -y curl git jq && \
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash - && \
    apt-get install -y nodejs && apt-get clean && rm -rf /var/lib/apt/lists/*
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
ENV PATH="/root/.cargo/bin:${PATH}"
RUN curl -L https://foundry.paradigm.xyz | bash && /root/.foundry/bin/foundryup
ENV PATH="/root/.foundry/bin:${PATH}"
ENV NPM_CONFIG_CACHE=/bowie/cache/npm
COPY agent/ /opt/bowie-agent/agent/
WORKDIR /opt/bowie-agent
RUN pip install --no-cache-dir -r agent/requirements.txt
ENTRYPOINT ["python", "-u", "-m", "agent"]
