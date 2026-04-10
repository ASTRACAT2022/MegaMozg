use std::env;
use std::thread;
use std::time::Duration;

fn main() {
    let args: Vec<String> = env::args().collect();
    let node_id = find_arg_value(&args, "--node-id").unwrap_or_else(|| "unknown-node".to_string());
    let region = find_arg_value(&args, "--region").unwrap_or_else(|| "unknown-region".to_string());

    println!("meza-node starting");
    println!("node_id={node_id}");
    println!("region={region}");
    println!("mode=scaffold");
    println!("next_steps=enrollment, heartbeat, signed-jobs");

    thread::sleep(Duration::from_millis(50));
}

fn find_arg_value(args: &[String], key: &str) -> Option<String> {
    args.iter()
        .position(|arg| arg == key)
        .and_then(|idx| args.get(idx + 1).cloned())
}
