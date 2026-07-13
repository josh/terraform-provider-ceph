package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var (
	_ resource.Resource                = &RBDMirroringPoolPeerResource{}
	_ resource.ResourceWithImportState = &RBDMirroringPoolPeerResource{}
)

func newRBDMirroringPoolPeerResource() resource.Resource {
	return &RBDMirroringPoolPeerResource{}
}

type RBDMirroringPoolPeerResource struct {
	client *restapi.Client
}

type RBDMirroringPoolPeerResourceModel struct {
	PoolName    types.String `tfsdk:"pool_name"`
	ClusterName types.String `tfsdk:"cluster_name"`
	ClientID    types.String `tfsdk:"client_id"`
	MonHost     types.String `tfsdk:"mon_host"`
	Key         types.String `tfsdk:"key"`
	UUID        types.String `tfsdk:"uuid"`
	Direction   types.String `tfsdk:"direction"`
	MirrorUUID  types.String `tfsdk:"mirror_uuid"`
}

func (r *RBDMirroringPoolPeerResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rbd_mirroring_pool_peer"
}

func (r *RBDMirroringPoolPeerResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource manages an RBD mirroring peer of a pool. Peers are registered records only; no connectivity to the remote cluster is validated. The pool's mirroring mode must be enabled first — reference `ceph_rbd_mirroring_pool_mode` (e.g. `pool_name = ceph_rbd_mirroring_pool_mode.example.pool_name`) so peers are also destroyed before mirroring is disabled, which otherwise fails while peers exist.",
		Attributes: map[string]resourceSchema.Attribute{
			"pool_name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the pool. Changing requires destroying and recreating the peer.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cluster_name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the remote cluster (site). Must differ from the local cluster name and be unique among the pool's peers.",
				Required:            true,
			},
			"client_id": resourceSchema.StringAttribute{
				MarkdownDescription: "The CephX client id used to connect to the remote cluster, without the `client.` prefix.",
				Required:            true,
			},
			"mon_host": resourceSchema.StringAttribute{
				MarkdownDescription: "The monitor addresses of the remote cluster.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
			},
			"key": resourceSchema.StringAttribute{
				MarkdownDescription: "The CephX key for the remote client. Stored in the Terraform state.",
				Optional:            true,
				Computed:            true,
				Sensitive:           true,
				Default:             stringdefault.StaticString(""),
			},
			"uuid": resourceSchema.StringAttribute{
				MarkdownDescription: "The unique identifier of the peer.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"direction": resourceSchema.StringAttribute{
				MarkdownDescription: "The replication direction: `rx`, `tx` or `rx-tx`.",
				Computed:            true,
			},
			"mirror_uuid": resourceSchema.StringAttribute{
				MarkdownDescription: "The mirror uuid of the remote cluster, once connected.",
				Computed:            true,
			},
		},
	}
}

func (r *RBDMirroringPoolPeerResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*restapi.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *restapi.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *RBDMirroringPoolPeerResource) updateModelFromAPI(data *RBDMirroringPoolPeerResourceModel, peer *restapi.RBDMirroringPoolPeer) {
	data.UUID = types.StringValue(peer.UUID)
	data.ClusterName = types.StringValue(peer.ClusterName)
	data.ClientID = types.StringValue(peer.ClientID)
	data.MonHost = types.StringValue(peer.MonHost)
	data.Key = types.StringValue(peer.Key)
	data.Direction = types.StringValue(peer.Direction)
	data.MirrorUUID = types.StringValue(peer.MirrorUUID)
}

func (r *RBDMirroringPoolPeerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RBDMirroringPoolPeerResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	poolName := data.PoolName.ValueString()

	spec := restapi.RBDMirroringPoolPeerSpec{
		ClusterName: data.ClusterName.ValueString(),
		ClientID:    data.ClientID.ValueString(),
	}
	if data.MonHost.ValueString() != "" {
		spec.MonHost = data.MonHost.ValueStringPointer()
	}
	if data.Key.ValueString() != "" {
		spec.Key = data.Key.ValueStringPointer()
	}

	uuid, err := r.client.CreateRBDMirroringPoolPeer(ctx, poolName, spec)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create mirroring peer on pool '%s': %s", poolName, err),
		)
		return
	}

	peer, err := r.client.GetRBDMirroringPoolPeer(ctx, poolName, uuid)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read mirroring peer '%s' on pool '%s' after creation: %s", uuid, poolName, err),
		)
		return
	}

	r.updateModelFromAPI(&data, peer)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RBDMirroringPoolPeerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RBDMirroringPoolPeerResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	peer, err := r.client.GetRBDMirroringPoolPeer(ctx, data.PoolName.ValueString(), data.UUID.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read mirroring peer '%s' on pool '%s': %s", data.UUID.ValueString(), data.PoolName.ValueString(), err),
		)
		return
	}

	r.updateModelFromAPI(&data, peer)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RBDMirroringPoolPeerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data RBDMirroringPoolPeerResourceModel
	var state RBDMirroringPoolPeerResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	poolName := state.PoolName.ValueString()
	uuid := state.UUID.ValueString()

	spec := restapi.RBDMirroringPoolPeerSpec{
		ClusterName: data.ClusterName.ValueString(),
		ClientID:    data.ClientID.ValueString(),
		MonHost:     data.MonHost.ValueStringPointer(),
		Key:         data.Key.ValueStringPointer(),
	}

	err := r.client.UpdateRBDMirroringPoolPeer(ctx, poolName, uuid, spec)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to update mirroring peer '%s' on pool '%s': %s", uuid, poolName, err),
		)
		return
	}

	peer, err := r.client.GetRBDMirroringPoolPeer(ctx, poolName, uuid)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read mirroring peer '%s' on pool '%s' after update: %s", uuid, poolName, err),
		)
		return
	}

	r.updateModelFromAPI(&data, peer)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RBDMirroringPoolPeerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data RBDMirroringPoolPeerResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteRBDMirroringPoolPeer(ctx, data.PoolName.ValueString(), data.UUID.ValueString())
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete mirroring peer '%s' on pool '%s': %s", data.UUID.ValueString(), data.PoolName.ValueString(), err),
		)
		return
	}
}

func (r *RBDMirroringPoolPeerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected format: pool_name/peer_uuid, got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pool_name"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("uuid"), parts[1])...)
}
