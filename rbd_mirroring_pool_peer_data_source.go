package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var _ datasource.DataSource = &RBDMirroringPoolPeerDataSource{}

func newRBDMirroringPoolPeerDataSource() datasource.DataSource {
	return &RBDMirroringPoolPeerDataSource{}
}

type RBDMirroringPoolPeerDataSource struct {
	client *restapi.Client
}

type RBDMirroringPoolPeerDataSourceModel struct {
	PoolName    types.String `tfsdk:"pool_name"`
	UUID        types.String `tfsdk:"uuid"`
	ClusterName types.String `tfsdk:"cluster_name"`
	ClientID    types.String `tfsdk:"client_id"`
	MonHost     types.String `tfsdk:"mon_host"`
	Key         types.String `tfsdk:"key"`
	Direction   types.String `tfsdk:"direction"`
	MirrorUUID  types.String `tfsdk:"mirror_uuid"`
}

func (d *RBDMirroringPoolPeerDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rbd_mirroring_pool_peer"
}

func (d *RBDMirroringPoolPeerDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about an RBD mirroring peer of a pool.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"pool_name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the pool",
				Required:            true,
			},
			"uuid": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The unique identifier of the peer",
				Required:            true,
			},
			"cluster_name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the remote cluster",
				Computed:            true,
			},
			"client_id": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The CephX client id used to connect to the remote cluster",
				Computed:            true,
			},
			"mon_host": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The monitor addresses of the remote cluster",
				Computed:            true,
			},
			"key": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The CephX key for the remote client",
				Computed:            true,
				Sensitive:           true,
			},
			"direction": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The replication direction: `rx`, `tx` or `rx-tx`",
				Computed:            true,
			},
			"mirror_uuid": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The mirror uuid of the remote cluster, once connected",
				Computed:            true,
			},
		},
	}
}

func (d *RBDMirroringPoolPeerDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*restapi.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *restapi.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client
}

func (d *RBDMirroringPoolPeerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RBDMirroringPoolPeerDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	peer, err := d.client.GetRBDMirroringPoolPeer(ctx, data.PoolName.ValueString(), data.UUID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get mirroring peer '%s' on pool '%s' from Ceph API: %s", data.UUID.ValueString(), data.PoolName.ValueString(), err),
		)
		return
	}

	data.ClusterName = types.StringValue(peer.ClusterName)
	data.ClientID = types.StringValue(peer.ClientID)
	data.MonHost = types.StringValue(peer.MonHost)
	data.Key = types.StringValue(peer.Key)
	data.Direction = types.StringValue(peer.Direction)
	data.MirrorUUID = types.StringValue(peer.MirrorUUID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
