create table if not exists neutron_project(
    id char(36) primary key,
    webhook_type varchar(20),
    repo_url varchar(200)
);
create table if not exists neutron_job(
    id int primary key auto_increment,
    project_id char(36) not null,
    name varchar(128) not null,
    status varchar(512) not null
);
